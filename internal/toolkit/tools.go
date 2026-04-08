package toolkit

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type Policy struct {
	TempDirName           string
	AllowRunCommand       bool
	MissingPathAsExistsNo bool
	WriteAllowPrefixes    []string
	// AllowedRunCommandExact: 允许执行的 run_command 命令（忽略首尾空白，比较时不区分大小写）
	AllowedRunCommandExact []string
	// AllowedRunCommandPrefixes: 允许执行的 run_command 命令前缀（忽略首尾空白，比较时不区分大小写）
	AllowedRunCommandPrefixes []string
	EnablePythonRewrite       bool
	EnablePortTools           bool // 启用 check_port / kill_port 工具
	EnableWebTools            bool // 启用 web_search / fetch_url 工具
	EnableOfficeTools         bool // 启用 read_word / write_word / read_excel / write_excel / read_pdf / write_pdf
}

// dirOf returns the directory part of a file path.
func dirOf(p string) string {
	for i := len(p) - 1; i >= 0; i-- {
		if p[i] == '/' || p[i] == '\\' {
			return p[:i]
		}
	}
	return "."
}

func BuildTools(workspaceRoot string, timeoutSec int, policy Policy) ([]tool.BaseTool, error) {
	ft := &fileToolSet{workspaceRoot: workspaceRoot, policy: policy}
	fileTools, err := ft.Tools()
	if err != nil {
		return nil, err
	}
	all := make([]tool.BaseTool, 0, len(fileTools)+1)
	all = append(all, fileTools...)
	if policy.AllowRunCommand {
		ct := &commandToolSet{
			workspaceRoot: workspaceRoot,
			timeout:       time.Duration(max(timeoutSec, 1)) * time.Second,
			policy:        policy,
		}
		cmdTools, err := ct.Tools()
		if err != nil {
			return nil, err
		}
		all = append(all, cmdTools...)
	}
	if policy.EnablePortTools {
		pt := &portToolSet{}
		portTools, err := pt.Tools()
		if err != nil {
			return nil, err
		}
		all = append(all, portTools...)
	}
	if policy.EnableWebTools {
		wst := &webSearchToolSet{}
		wsTools, err := wst.Tools()
		if err != nil {
			return nil, err
		}
		all = append(all, wsTools...)

		fut := &fetchURLToolSet{}
		fuTools, err := fut.Tools()
		if err != nil {
			return nil, err
		}
		all = append(all, fuTools...)
	}
	if policy.EnableOfficeTools {
		officeTools, err := buildOfficeTools(workspaceRoot)
		if err != nil {
			return nil, err
		}
		all = append(all, officeTools...)
	}
	return all, nil
}

type fileToolSet struct {
	workspaceRoot string
	policy        Policy
}

type listDirInput struct {
	Path string `json:"path"`
}
type listDirOutput struct {
	Path    string   `json:"path"`
	Exists  bool     `json:"exists"`
	Entries []string `json:"entries"`
}

func (f *fileToolSet) listDir(_ context.Context, in listDirInput) (*listDirOutput, error) {
	target, err := resolveSafePath(f.workspaceRoot, in.Path)
	if err != nil {
		return nil, err
	}
	items, err := os.ReadDir(target)
	if err != nil {
		if os.IsNotExist(err) {
			return &listDirOutput{Path: target, Exists: false, Entries: []string{}}, nil
		}
		return nil, fmt.Errorf("read dir: %w", err)
	}
	out := &listDirOutput{Path: target, Exists: true, Entries: make([]string, 0, len(items))}
	for _, item := range items {
		name := item.Name()
		if item.IsDir() {
			name += "/"
		}
		out.Entries = append(out.Entries, name)
	}
	return out, nil
}

type readFileInput struct {
	Path string `json:"path"`
}
type readFileOutput struct {
	Path    string `json:"path"`
	Exists  bool   `json:"exists"`
	Content string `json:"content"`
}

func (f *fileToolSet) readFile(_ context.Context, in readFileInput) (*readFileOutput, error) {
	target, err := resolveSafePath(f.workspaceRoot, in.Path)
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(target)
	if err != nil {
		if os.IsNotExist(err) {
			return &readFileOutput{Path: target, Exists: false, Content: ""}, nil
		}
		return nil, fmt.Errorf("read file: %w", err)
	}
	return &readFileOutput{Path: target, Exists: true, Content: string(data)}, nil
}

type writeFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}
type writeFileOutput struct {
	Path    string `json:"path"`
	Bytes   int    `json:"bytes"`
	Created bool   `json:"created"`
}

func (f *fileToolSet) enforceWritePolicy(target string) error {
	if len(f.policy.WriteAllowPrefixes) == 0 {
		return nil
	}
	rel, err := filepath.Rel(f.workspaceRoot, target)
	if err != nil {
		return err
	}
	rel = filepath.ToSlash(rel)
	for _, p := range f.policy.WriteAllowPrefixes {
		pp := strings.TrimSuffix(filepath.ToSlash(p), "/")
		if rel == pp || strings.HasPrefix(rel, pp+"/") {
			return nil
		}
	}
	return fmt.Errorf("write target not allowed by policy: %s", rel)
}

func (f *fileToolSet) writeFile(_ context.Context, in writeFileInput) (*writeFileOutput, error) {
	target, err := resolveSafePath(f.workspaceRoot, in.Path)
	if err != nil {
		return nil, err
	}
	if err := f.enforceWritePolicy(target); err != nil {
		return nil, err
	}
	if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
		return nil, fmt.Errorf("create parent directory: %w", err)
	}
	_, statErr := os.Stat(target)
	created := os.IsNotExist(statErr)
	if err := os.WriteFile(target, []byte(in.Content), 0o644); err != nil {
		return nil, fmt.Errorf("write file: %w", err)
	}
	return &writeFileOutput{Path: target, Bytes: len(in.Content), Created: created}, nil
}

type writeTempFileInput struct {
	Path    string `json:"path"`
	Content string `json:"content"`
}

func (f *fileToolSet) writeTempFile(ctx context.Context, in writeTempFileInput) (*writeFileOutput, error) {
	tempName := f.policy.TempDirName
	if tempName == "" {
		tempName = ".build-agent-tmp"
	}
	tempRoot := filepath.Join(f.workspaceRoot, tempName)
	tempTools := &fileToolSet{workspaceRoot: tempRoot}
	return tempTools.writeFile(ctx, writeFileInput{Path: in.Path, Content: in.Content})
}

func (f *fileToolSet) Tools() ([]tool.BaseTool, error) {
	listTool, err := toolutils.InferTool("list_dir", "列出目录内容", f.listDir)
	if err != nil {
		return nil, err
	}
	readTool, err := toolutils.InferTool("read_file", "读取文件内容", f.readFile)
	if err != nil {
		return nil, err
	}
	writeTool, err := toolutils.InferTool("write_file", "写入文件", f.writeFile)
	if err != nil {
		return nil, err
	}
	writeTempTool, err := toolutils.InferTool("write_temp_file", "写入临时文件", f.writeTempFile)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{listTool, readTool, writeTool, writeTempTool}, nil
}

type commandToolSet struct {
	workspaceRoot string
	timeout       time.Duration
	policy        Policy
}
type runCommandInput struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
}
type runCommandOutput struct {
	Cwd      string `json:"cwd"`
	Command  string `json:"command"`
	ExitCode int    `json:"exit_code"`
	Stdout   string `json:"stdout"`
	Stderr   string `json:"stderr"`
}

type startBackgroundInput struct {
	Command string `json:"command"`
	Cwd     string `json:"cwd"`
	LogPath string `json:"log_path"`
}
type startBackgroundOutput struct {
	Cwd     string `json:"cwd"`
	Command string `json:"command"`
	PID     int    `json:"pid"`
	LogPath string `json:"log_path"`
	Note    string `json:"note"`
}

func (c *commandToolSet) startBackgroundCmd(_ context.Context, in startBackgroundInput) (*startBackgroundOutput, error) {
	if strings.TrimSpace(in.Command) == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}
	cwd, err := resolveSafePath(c.workspaceRoot, in.Cwd)
	if err != nil {
		return nil, err
	}
	logPath := in.LogPath
	if logPath == "" {
		tmpDir := c.policy.TempDirName
		if tmpDir == "" {
			tmpDir = ".build-agent-tmp"
		}
		logPath = filepath.ToSlash(filepath.Join(c.workspaceRoot, tmpDir, "server.log"))
	} else if !filepath.IsAbs(logPath) {
		logPath = filepath.ToSlash(filepath.Join(c.workspaceRoot, logPath))
	}
	if err := c.enforceRunCommandPolicy(strings.TrimSpace(in.Command)); err != nil {
		return nil, err
	}
	pid, note, err := startBackground(in.Command, cwd, logPath)
	if err != nil {
		return nil, err
	}
	fmt.Fprintf(os.Stderr, "[build-agent] start_background: %s\n", note)
	return &startBackgroundOutput{
		Cwd: cwd, Command: in.Command, PID: pid, LogPath: logPath, Note: note,
	}, nil
}

func (c *commandToolSet) runCommand(ctx context.Context, in runCommandInput) (*runCommandOutput, error) {
	if strings.TrimSpace(in.Command) == "" {
		return nil, fmt.Errorf("command cannot be empty")
	}
	cwd, err := resolveSafePath(c.workspaceRoot, in.Cwd)
	if err != nil {
		return nil, err
	}
	runCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	effective := strings.TrimSpace(in.Command)
	if err := c.enforceRunCommandPolicy(effective); err != nil {
		return nil, err
	}
	// Reject commands that are known to block indefinitely — force use of start_background.
	if isBlockingServiceCommand(effective) {
		return nil, fmt.Errorf(
			"[build-agent] 检测到阻塞型服务启动命令，run_command 不支持此类命令（会永久阻塞）。\n"+
				"请改用 start_background 工具启动服务，例如：\n"+
				"  start_background(command=%q, cwd=..., log_path=\".eval-agent-tmp/server.log\")",
			effective,
		)
	}
	rewriteNote := ""
	if c.policy.EnablePythonRewrite {
		if alt, note, ok := tryRewritePythonServerForeground(effective, c.workspaceRoot); ok {
			effective = alt
			rewriteNote = note
			fmt.Fprintf(os.Stderr, "%s\n", rewriteNote)
		}
	}
	if runtime.GOOS == "windows" {
		if alt, note, ok := rewriteWindowsStartToPS(effective); ok {
			effective = alt
			if rewriteNote != "" {
				rewriteNote += note
			} else {
				rewriteNote = note
			}
			fmt.Fprint(os.Stderr, note)
		}
	}
	cmd := makeShellCommand(runCtx, effective)
	cmd.Dir = cwd
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if rewriteNote != "" {
		stderr.WriteString(rewriteNote + "\n")
	}
	fmt.Fprintf(os.Stderr, "[build-agent] run_command 开始执行（最长约 %v）\ncwd: %s\ncommand: %s\n", c.timeout, cwd, effective)
	err = cmd.Run()
	exitCode := 0
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) || runCtx.Err() == context.DeadlineExceeded {
			exitCode = 124
			stderr.WriteString("\n[build-agent] 命令已超时，exit_code=124，请尝试后台启动并写日志，再做短探测。\n")
		} else if ee, ok := err.(*exec.ExitError); ok {
			exitCode = ee.ExitCode()
		} else {
			return nil, fmt.Errorf("run command: %w", err)
		}
	}
	return &runCommandOutput{
		Cwd: cwd, Command: effective, ExitCode: exitCode, Stdout: stdout.String(), Stderr: stderr.String(),
	}, nil
}

func (c *commandToolSet) enforceRunCommandPolicy(effective string) error {
	if len(c.policy.AllowedRunCommandExact) == 0 && len(c.policy.AllowedRunCommandPrefixes) == 0 {
		return nil
	}
	normalized := strings.ToLower(strings.TrimSpace(effective))
	for _, ex := range c.policy.AllowedRunCommandExact {
		if normalized == strings.ToLower(strings.TrimSpace(ex)) {
			return nil
		}
	}
	for _, pre := range c.policy.AllowedRunCommandPrefixes {
		preN := strings.ToLower(strings.TrimSpace(pre))
		if preN == "" {
			continue
		}
		if strings.HasPrefix(normalized, preN) {
			return nil
		}
	}
	return fmt.Errorf("run_command not allowed by policy: %s", effective)
}

func (c *commandToolSet) Tools() ([]tool.BaseTool, error) {
	runTool, err := toolutils.InferTool("run_command", "执行命令（同步等待结束）", c.runCommand)
	if err != nil {
		return nil, err
	}
	bgTool, err := toolutils.InferTool(
		"start_background",
		"后台启动长时间运行的服务进程（立即返回，输出写入 log_path）。适用于 HTTP 服务、Spring Boot、npm start 等不会自行退出的命令。",
		c.startBackgroundCmd,
	)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{runTool, bgTool}, nil
}

// makeShellCommand builds the exec.Cmd for the given shell command string.
// On Windows we use PowerShell to avoid cmd.exe quote-parsing issues and "file not found" popups.
func makeShellCommand(ctx context.Context, command string) *exec.Cmd {
	if runtime.GOOS == "windows" {
		return exec.CommandContext(ctx, "powershell", "-NoProfile", "-NonInteractive", "-Command", command)
	}
	return exec.CommandContext(ctx, "sh", "-lc", command)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

const evalServerLogName = "eval-server.log"

// reWindowsStartNoTitle matches `start /SWITCH` without a preceding title argument.
var reWindowsStartNoTitle = regexp.MustCompile(`(?i)(\bstart\b)(\s+)(/)`)

// reWindowsStartBCmd matches: start "" /B cmd /c "inner"
var reWindowsStartBCmd = regexp.MustCompile(`(?i)^start\s+""\s+/[Bb]\s+cmd\s+/[cC]\s+"(.*)"$`)

// reWindowsStartBBat matches: start "" /B "path\to.bat"
var reWindowsStartBBat = regexp.MustCompile(`(?i)^start\s+""\s+/[Bb]\s+"([^"]+\.bat)"$`)

// reWindowsStartBBare matches: start "" /B program [args...]
var reWindowsStartBBare = regexp.MustCompile(`(?i)^start\s+""\s+/[Bb]\s+(.+)$`)

// fixWindowsStartEmptyTitle inserts "" title after start when the next token is a /switch.
func fixWindowsStartEmptyTitle(cmd string) string {
	return reWindowsStartNoTitle.ReplaceAllString(cmd, `${1}${2}"" ${3}`)
}

// rewriteWindowsStartToPS rewrites any `start "" /B ...` background-launch pattern into
// a PowerShell Start-Process command, avoiding all cmd.exe quote/popup issues.
func rewriteWindowsStartToPS(command string) (rewritten string, note string, ok bool) {
	s := fixWindowsStartEmptyTitle(strings.TrimSpace(command))

	// Pattern 1: start "" /B cmd /c "inner command"
	if m := reWindowsStartBCmd.FindStringSubmatch(s); m != nil {
		inner := strings.ReplaceAll(m[1], `\\`, `\`)
		inner = strings.ReplaceAll(inner, `"`, `\"`)
		rewritten = fmt.Sprintf(`Start-Process powershell -ArgumentList '-NoProfile','-NonInteractive','-Command',"%s" -NoNewWindow`, inner)
		note = "[build-agent] Windows: 已将 start+cmd/c 改写为 Start-Process，避免引号嵌套弹窗。\n"
		return rewritten, note, true
	}

	// Pattern 2: start "" /B "bat-path"
	if m := reWindowsStartBBat.FindStringSubmatch(s); m != nil {
		batPath := strings.ReplaceAll(m[1], `\\`, `\`)
		rewritten = fmt.Sprintf(`Start-Process -FilePath "%s" -NoNewWindow`, batPath)
		note = "[build-agent] Windows: 已将 start+bat 改写为 Start-Process，避免弹窗。\n"
		return rewritten, note, true
	}

	// Pattern 3: start "" /B program [args...]
	if m := reWindowsStartBBare.FindStringSubmatch(s); m != nil {
		rest := strings.TrimSpace(m[1])
		parts := strings.SplitN(rest, " ", 2)
		if len(parts) == 2 {
			args := strings.ReplaceAll(parts[1], `"`, `\"`)
			rewritten = fmt.Sprintf(`Start-Process -FilePath "%s" -ArgumentList "%s" -NoNewWindow`, parts[0], args)
		} else {
			rewritten = fmt.Sprintf(`Start-Process -FilePath "%s" -NoNewWindow`, parts[0])
		}
		note = "[build-agent] Windows: 已将 start /B 改写为 Start-Process，避免弹窗。\n"
		return rewritten, note, true
	}

	return "", "", false
}

var rePythonServerForeground = regexp.MustCompile(`(?i)^((?:py|python|python3))\s+(\S+\.py)(?:\s+(\d+))?\s*$`)

// blockingServicePatterns matches commands that start long-running services and will never exit.
var blockingServicePatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?(java\s+(-\S+\s+)*-jar\b)`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?mvn\s+(spring-boot:run|exec:java)\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?npm\s+(start|run\s+dev|run\s+start)\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?yarn\s+(start|dev)\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?pnpm\s+(start|dev)\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?uvicorn\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?gunicorn\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?flask\s+run\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?go\s+run\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?dotnet\s+run\b`),
	regexp.MustCompile(`(?i)^(powershell\s+(-\S+\s+)*-Command\s+.*)?cargo\s+run\b`),
}

// shortCommandSuffixes are argument patterns that indicate a command exits quickly
// and should NOT be treated as a blocking service.
var shortCommandSuffixes = regexp.MustCompile(`(?i)\s+(--version|-version|--help|-h|-v|--v)\s*$`)

// isBlockingServiceCommand returns true if the command is a known long-running service
// that will never exit on its own and must be run via start_background instead.
func isBlockingServiceCommand(cmd string) bool {
	s := strings.TrimSpace(cmd)
	// Already backgrounded via redirection or nohup — allow through.
	if isLikelyAlreadyBackgroundOrRedirected(s) {
		return false
	}
	// Commands ending with version/help flags exit immediately — allow through.
	if shortCommandSuffixes.MatchString(s) {
		return false
	}
	for _, re := range blockingServicePatterns {
		if re.MatchString(s) {
			return true
		}
	}
	return false
}

func isLikelyAlreadyBackgroundOrRedirected(cmd string) bool {
	s := strings.TrimSpace(cmd)
	lower := strings.ToLower(s)
	if strings.Contains(s, ">") || strings.Contains(lower, evalServerLogName) {
		return true
	}
	if runtime.GOOS == "windows" {
		return strings.Contains(lower, `start ""`) ||
			(strings.Contains(lower, "start ") && strings.Contains(lower, "/b")) ||
			strings.Contains(lower, "start-process") ||
			strings.Contains(lower, "start_background")
	}
	return strings.Contains(lower, "nohup ") ||
		strings.HasSuffix(strings.TrimSpace(s), "&") ||
		strings.Contains(lower, "start_background")
}

func tryRewritePythonServerForeground(raw string, workspaceRoot string) (rewritten string, note string, ok bool) {
	raw = strings.TrimSpace(raw)
	if isLikelyAlreadyBackgroundOrRedirected(raw) {
		return "", "", false
	}
	m := rePythonServerForeground.FindStringSubmatch(raw)
	if m == nil {
		return "", "", false
	}
	pyexe := m[1]
	script := m[2]
	port := 8000
	portNote := ""
	if len(m) >= 4 && m[3] != "" {
		p, err := strconv.Atoi(m[3])
		if err != nil {
			return "", "", false
		}
		if p == 0 {
			port = 8000
			portNote = "端口 0 无效，已改为默认 8000。"
		} else if p >= 1 && p <= 65535 {
			port = p
		} else {
			return "", "", false
		}
	}
	tmpDir := filepath.Join(workspaceRoot, ".eval-agent-tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return "", "", false
	}
	logAbs := filepath.ToSlash(filepath.Join(tmpDir, evalServerLogName))
	if runtime.GOOS == "windows" {
		// Wrap in a child powershell so Start-Process returns immediately (no -Redirect* which blocks).
		inner := fmt.Sprintf(`%s %s %d > '%s' 2>&1`, pyexe, script, port, logAbs)
		rewritten = fmt.Sprintf(`Start-Process powershell -ArgumentList '-NoProfile','-NonInteractive','-Command',"%s" -NoNewWindow`, strings.ReplaceAll(inner, `"`, `\"`))
	} else {
		rewritten = fmt.Sprintf("nohup %s %s %d > %s 2>&1 &", pyexe, script, port, logAbs)
	}
	note = fmt.Sprintf("[build-agent] 已将前台 python 服务改写为后台，日志：%s", logAbs)
	if portNote != "" {
		note += " " + portNote
	}
	return rewritten, note, true
}
