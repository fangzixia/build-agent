//go:build !windows

package toolkit

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
)

// startBackground launches command as a background process on Unix,
// redirecting stdout+stderr to logPath. Returns immediately without waiting.
func startBackground(command, cwd, logPath string) (pid int, note string, err error) {
	if err := os.MkdirAll(dirOf(logPath), 0o755); err != nil {
		return 0, "", fmt.Errorf("create log dir: %w", err)
	}
	logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return 0, "", fmt.Errorf("open log file: %w", err)
	}
	// Do NOT defer logFile.Close() — close after child exits.

	// Strip trailing & if present — we handle backgrounding ourselves.
	command = strings.TrimRight(strings.TrimSpace(command), "&")
	cmd := exec.Command("sh", "-c", command)
	cmd.Dir = cwd
	cmd.Stdout = logFile
	cmd.Stderr = logFile
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
