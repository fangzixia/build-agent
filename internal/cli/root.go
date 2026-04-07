package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"build-agent/internal/config"
	"build-agent/internal/core"
	httpserver "build-agent/internal/http"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	cmd := &cobra.Command{Use: "agent", Short: "build-agent unified agent"}
	cmd.AddCommand(
		newAgentCmd("code", "code"),
		newAgentCmd("analysis", "analysis"),
		newAgentCmd("eval", "eval"),
		newAgentCmd("req", "requirements", "requirements"),
	)
	cmd.AddCommand(newBuildCmd())
	cmd.AddCommand(newServeCmd())
	cmd.AddCommand(newDesktopCmd())
	return cmd
}

func newBuildCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "build",
		Short: "基于 requirements 的 code/eval 循环构建",
	}
	cmd.AddCommand(newBuildRunCmd())
	cmd.AddCommand(newBuildChatCmd())
	return cmd
}

func newAgentCmd(use, agentName string, aliases ...string) *cobra.Command {
	cmd := &cobra.Command{
		Use:     use,
		Aliases: aliases,
		Short:   "固定场景命令",
	}
	cmd.AddCommand(newRunCmdForAgent(agentName))
	cmd.AddCommand(newChatCmdForAgent(agentName))
	cmd.AddCommand(newServeCmdForAgent(agentName))
	return cmd
}

func newRunCmdForAgent(agentName string) *cobra.Command {
	var task string
	var noColor, noSpinner bool
	c := &cobra.Command{
		Use:   "run",
		Short: "单次执行任务",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			svc, err := core.NewService(cmd.Context(), cfg, agentName)
			if err != nil {
				return err
			}
			renderer := newCLIRenderer(!noColor, !noSpinner)
			renderer.StartSpinner("正在思考与执行")
			result, err := svc.RunTaskWithProgress(cmd.Context(), task, renderer.PrintEvent)
			renderer.StopSpinner()
			success := err == nil
			if success && result != nil {
				success = !result.HasError
			}
			renderer.PrintDoneState(success)
			if err != nil {
				return err
			}
			fmt.Printf("\n最终结果：\n%s\n", result.Output)
			return nil
		},
	}
	c.Flags().StringVar(&task, "task", "", "任务描述")
	c.Flags().BoolVar(&noColor, "no-color", false, "禁用彩色输出")
	c.Flags().BoolVar(&noSpinner, "no-spinner", false, "禁用 spinner 动画")
	return c
}

func newChatCmdForAgent(agentName string) *cobra.Command {
	var noColor, noSpinner bool
	c := &cobra.Command{
		Use:   "chat",
		Short: "交互模式",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			svc, err := core.NewService(cmd.Context(), cfg, agentName)
			if err != nil {
				return err
			}
			fmt.Println("进入 chat 模式，输入 exit 退出。")
			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("> ")
				line, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				task := strings.TrimSpace(line)
				if task == "exit" || task == "quit" {
					return nil
				}
				renderer := newCLIRenderer(!noColor, !noSpinner)
				start := time.Now()
				renderer.StartSpinner("正在思考与执行")
				result, err := svc.RunTaskWithProgress(cmd.Context(), task, renderer.PrintEvent)
				renderer.StopSpinner()
				success := err == nil
				if success && result != nil {
					success = !result.HasError
				}
				renderer.PrintDoneState(success)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					continue
				}
				fmt.Printf("\n[完成] 耗时 %s\n%s\n\n", time.Since(start).Round(time.Second), result.Output)
			}
		},
	}
	c.Flags().BoolVar(&noColor, "no-color", false, "禁用彩色输出")
	c.Flags().BoolVar(&noSpinner, "no-spinner", false, "禁用 spinner 动画")
	return c
}

func newBuildRunCmd() *cobra.Command {
	var task, requirementsPath string
	var noColor, noSpinner bool
	c := &cobra.Command{
		Use:   "run",
		Short: "执行 build 编排流程",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			svc := core.NewBuildService(cfg)
			renderer := newCLIRenderer(!noColor, !noSpinner)
			renderer.StartSpinner("正在执行 build 编排")
			result, err := svc.RunBuildTask(cmd.Context(), task, requirementsPath, renderer.PrintEvent)
			renderer.StopSpinner()
			success := err == nil
			if success && result != nil {
				success = !result.HasError
			}
			renderer.PrintDoneState(success)
			if err != nil {
				return err
			}
			fmt.Printf("\n最终结果：\n%s\n", result.Output)
			return nil
		},
	}
	c.Flags().StringVar(&task, "task", "", "任务描述")
	c.Flags().StringVar(&requirementsPath, "requirements-path", "", "需求文件路径（默认使用最新 .spec/REQ-xxxxx.md）")
	c.Flags().BoolVar(&noColor, "no-color", false, "禁用彩色输出")
	c.Flags().BoolVar(&noSpinner, "no-spinner", false, "禁用 spinner 动画")
	return c
}

func newBuildChatCmd() *cobra.Command {
	var requirementsPath string
	var noColor, noSpinner bool
	c := &cobra.Command{
		Use:   "chat",
		Short: "build 交互模式",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			svc := core.NewBuildService(cfg)
			fmt.Println("进入 build chat 模式，输入 exit 退出。")
			reader := bufio.NewReader(os.Stdin)
			for {
				fmt.Print("> ")
				line, err := reader.ReadString('\n')
				if err != nil {
					return err
				}
				task := strings.TrimSpace(line)
				if task == "exit" || task == "quit" {
					return nil
				}
				renderer := newCLIRenderer(!noColor, !noSpinner)
				start := time.Now()
				renderer.StartSpinner("正在执行 build 编排")
				result, err := svc.RunBuildTask(cmd.Context(), task, requirementsPath, renderer.PrintEvent)
				renderer.StopSpinner()
				success := err == nil
				if success && result != nil {
					success = !result.HasError
				}
				renderer.PrintDoneState(success)
				if err != nil {
					fmt.Printf("error: %v\n", err)
					continue
				}
				fmt.Printf("\n[完成] 耗时 %s\n%s\n\n", time.Since(start).Round(time.Second), result.Output)
			}
		},
	}
	c.Flags().StringVar(&requirementsPath, "requirements-path", "", "需求文件路径（默认使用最新 .spec/REQ-xxxxx.md）")
	c.Flags().BoolVar(&noColor, "no-color", false, "禁用彩色输出")
	c.Flags().BoolVar(&noSpinner, "no-spinner", false, "禁用 spinner 动画")
	return c
}

func newServeCmd() *cobra.Command {
	var addr string
	c := &cobra.Command{
		Use:   "serve",
		Short: "启动 HTTP 服务",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if addr != "" {
				cfg.Base.HTTPAddr = addr
			}
			server := httpserver.New(cfg, "code")
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			fmt.Printf("HTTP server listening on %s\n", cfg.Base.HTTPAddr)
			return server.ListenAndServe(ctx, cfg.Base.HTTPAddr)
		},
	}
	c.Flags().StringVar(&addr, "addr", "", "HTTP 监听地址")
	return c
}

func newDesktopCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "desktop",
		Short: "启动桌面应用模式",
		RunE: func(cmd *cobra.Command, _ []string) error {
			return fmt.Errorf("desktop command must be run from cmd/desktop/main.go binary")
		},
	}
}

func newServeCmdForAgent(agentName string) *cobra.Command {
	var addr string
	c := &cobra.Command{
		Use:   "serve",
		Short: "启动 HTTP 服务（固定场景默认路由）",
		RunE: func(cmd *cobra.Command, _ []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if addr != "" {
				cfg.Base.HTTPAddr = addr
			}
			server := httpserver.New(cfg, agentName)
			ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
			defer stop()
			fmt.Printf("HTTP server listening on %s (default agent: %s)\n", cfg.Base.HTTPAddr, agentName)
			return server.ListenAndServe(ctx, cfg.Base.HTTPAddr)
		},
	}
	c.Flags().StringVar(&addr, "addr", "", "HTTP 监听地址")
	return c
}

type cliRenderer struct {
	mu        sync.Mutex
	stopCh    chan struct{}
	doneCh    chan struct{}
	running   bool
	color     bool
	spinner   bool
	iterInfo  string
	lastError string
}

func newCLIRenderer(enableColor, enableSpinner bool) *cliRenderer {
	tty := isInteractiveTerminal()
	return &cliRenderer{color: enableColor && tty, spinner: enableSpinner && tty}
}

func (r *cliRenderer) StartSpinner(hint string) {
	if !r.spinner {
		return
	}
	r.mu.Lock()
	if r.running {
		r.mu.Unlock()
		return
	}
	r.stopCh = make(chan struct{})
	r.doneCh = make(chan struct{})
	r.running = true
	r.mu.Unlock()
	go func() {
		defer close(r.doneCh)
		frames := []string{"-", "\\", "|", "/"}
		ticker := time.NewTicker(120 * time.Millisecond)
		defer ticker.Stop()
		i := 0
		for {
			select {
			case <-r.stopCh:
				return
			case <-ticker.C:
				r.mu.Lock()
				fmt.Printf("\r\033[2K%s %s", frames[i%len(frames)], hint)
				r.mu.Unlock()
				i++
			}
		}
	}()
}

func (r *cliRenderer) StopSpinner() {
	if !r.spinner {
		return
	}
	r.mu.Lock()
	if !r.running {
		r.mu.Unlock()
		return
	}
	close(r.stopCh)
	doneCh := r.doneCh
	r.running = false
	r.mu.Unlock()
	<-doneCh
	fmt.Print("\r\033[2K\n")
}

func (r *cliRenderer) PrintEvent(ev core.EventLog) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if iter, ok := parseIterationInfo(ev.Output); ok {
		r.iterInfo = iter
	}
	if ev.Error != "" {
		r.lastError = ev.Error
	}
	prefix := ""
	if r.iterInfo != "" {
		prefix = r.iterInfo + " "
	}
	if ev.Error != "" {
		fmt.Printf("%s[error][%s] %s\n", prefix, ev.AgentName, oneLine(ev.Error))
		return
	}
	if ev.ToolName == "run_command" {
		fmt.Printf("%s[tool][run_command]\n%s\n", prefix, formatRunCommandFull(ev))
		return
	}
	content := summarizeEventOutput(ev)
	switch {
	case ev.ToolName != "":
		fmt.Printf("%s[tool][%s] %s\n", prefix, ev.ToolName, content)
	case ev.AgentName == "system":
		fmt.Printf("%s[status] %s\n", prefix, content)
	default:
		fmt.Printf("%s[agent][%s] %s\n", prefix, ev.AgentName, content)
	}
}

func (r *cliRenderer) PrintDoneState(success bool) {
	if success {
		fmt.Printf("[done] 执行已结束\n")
		return
	}
	if r.lastError != "" {
		fmt.Printf("[done] 执行结束（有错误） 最后错误：%s\n", oneLine(r.lastError))
		return
	}
	fmt.Printf("[done] 执行结束（有错误）\n")
}

func parseIterationInfo(output string) (string, bool) {
	text := strings.TrimSpace(output)
	if !strings.HasPrefix(text, "iter ") {
		return "", false
	}
	var exCur, exMax, peCur, peMax int
	if _, err := fmt.Sscanf(text, "iter exec=%d/%d plan=%d/%d", &exCur, &exMax, &peCur, &peMax); err != nil {
		return "", false
	}
	return fmt.Sprintf("[iter exec %d/%d | plan %d/%d]", exCur, exMax, peCur, peMax), true
}

func summarizeEventOutput(ev core.EventLog) string {
	trimmed := strings.TrimSpace(ev.Output)
	if trimmed == "" {
		return ""
	}
	if !strings.HasPrefix(trimmed, "{") {
		return oneLine(trimmed)
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return oneLine(trimmed)
	}
	if resp, ok := obj["response"].(string); ok && strings.TrimSpace(resp) != "" {
		return oneLine(resp)
	}
	return oneLine(trimmed)
}

func formatRunCommandFull(ev core.EventLog) string {
	trimmed := strings.TrimSpace(ev.Output)
	if trimmed == "" {
		return "(无输出)"
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(trimmed), &obj); err != nil {
		return trimmed
	}
	cwd := strings.TrimSpace(fmt.Sprint(obj["cwd"]))
	cmd := strings.TrimSpace(fmt.Sprint(obj["command"]))
	code := fmt.Sprint(obj["exit_code"])
	out := strings.TrimSpace(fmt.Sprint(obj["stdout"]))
	errOut := strings.TrimSpace(fmt.Sprint(obj["stderr"]))
	var b strings.Builder
	b.WriteString("cwd: " + cwd + "\n")
	b.WriteString("command: " + cmd + "\n")
	b.WriteString("exit_code: " + code + "\n")
	b.WriteString("--- stderr ---\n")
	if errOut == "" || errOut == "<nil>" {
		b.WriteString("(空)\n")
	} else {
		b.WriteString(errOut + "\n")
	}
	b.WriteString("--- stdout ---\n")
	if out == "" {
		b.WriteString("(空)")
	} else {
		b.WriteString(out)
	}
	return b.String()
}

func oneLine(s string) string {
	out := strings.TrimSpace(s)
	out = strings.ReplaceAll(out, "\r\n", " ")
	out = strings.ReplaceAll(out, "\n", " ")
	return out
}

func isInteractiveTerminal() bool {
	fi, err := os.Stdout.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
