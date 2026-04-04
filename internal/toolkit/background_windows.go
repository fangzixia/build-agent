//go:build windows

package toolkit

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

// startBackground launches command as a background process on Windows,
// redirecting stdout+stderr to logPath. Returns immediately without waiting.
func startBackground(command, cwd, logPath string) (pid int, note string, err error) {
	if err := os.MkdirAll(dirOf(logPath), 0o755); err != nil {
		return 0, "", fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, "", fmt.Errorf("open log file: %w", err)
	}
	// Do NOT defer logFile.Close() — the child process needs the handle to stay open.
	// It will be closed after the child exits via the goroutine below.

	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	cmd.Dir = cwd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
	cmd.SysProcAttr = &syscall.SysProcAttr{
		// CREATE_NEW_PROCESS_GROUP: isolate signals; CREATE_NO_WINDOW: no console popup.
		// Do NOT use DETACHED_PROCESS — it prevents handle inheritance, breaking stdout redirect.
		CreationFlags: 0x00000200 | 0x08000000, // CREATE_NEW_PROCESS_GROUP | CREATE_NO_WINDOW
	}
	if err := cmd.Start(); err != nil {
		logFile.Close()
		return 0, "", fmt.Errorf("start background process: %w", err)
	}
	go func() {
		_ = cmd.Wait()
		logFile.Close()
	}()
	note = fmt.Sprintf("[build-agent] 已后台启动 (pid=%d)，日志：%s", cmd.Process.Pid, logPath)
	return cmd.Process.Pid, note, nil
}
