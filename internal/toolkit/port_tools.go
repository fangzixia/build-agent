package toolkit

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"runtime"
	"strconv"
	"strings"
	"time"

	"github.com/cloudwego/eino/components/tool"
	toolutils "github.com/cloudwego/eino/components/tool/utils"
)

type portToolSet struct{}

type checkPortInput struct {
	Port int `json:"port"`
}
type checkPortOutput struct {
	Port      int    `json:"port"`
	InUse     bool   `json:"in_use"`
	PIDs      []int  `json:"pids"`
	Processes string `json:"processes"`
}

type killPortInput struct {
	Port  int  `json:"port"`
	Force bool `json:"force"`
}
type killPortOutput struct {
	Port    int    `json:"port"`
	Killed  []int  `json:"killed"`
	Message string `json:"message"`
}

func (p *portToolSet) checkPort(_ context.Context, in checkPortInput) (*checkPortOutput, error) {
	if in.Port < 1 || in.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", in.Port)
	}
	// Quick TCP dial to check if port is open
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", in.Port), 300*time.Millisecond)
	inUse := err == nil
	if conn != nil {
		conn.Close()
	}
	out := &checkPortOutput{Port: in.Port, InUse: inUse}
	if inUse {
		pids, procs := pidsByPort(in.Port)
		out.PIDs = pids
		out.Processes = procs
	}
	return out, nil
}

func (p *portToolSet) killPort(_ context.Context, in killPortInput) (*killPortOutput, error) {
	if in.Port < 1 || in.Port > 65535 {
		return nil, fmt.Errorf("invalid port: %d", in.Port)
	}
	pids, _ := pidsByPort(in.Port)
	if len(pids) == 0 {
		return &killPortOutput{Port: in.Port, Killed: []int{}, Message: "no process found on port"}, nil
	}
	killed := make([]int, 0, len(pids))
	var errs []string
	for _, pid := range pids {
		if err := killPID(pid, in.Force); err != nil {
			errs = append(errs, fmt.Sprintf("pid %d: %v", pid, err))
		} else {
			killed = append(killed, pid)
		}
	}
	msg := fmt.Sprintf("killed %d process(es) on port %d", len(killed), in.Port)
	if len(errs) > 0 {
		msg += "; errors: " + strings.Join(errs, "; ")
	}
	return &killPortOutput{Port: in.Port, Killed: killed, Message: msg}, nil
}

func (p *portToolSet) Tools() ([]tool.BaseTool, error) {
	checkTool, err := toolutils.InferTool("check_port", "检查端口是否被占用，返回占用进程信息", p.checkPort)
	if err != nil {
		return nil, err
	}
	killTool, err := toolutils.InferTool("kill_port", "终止占用指定端口的进程", p.killPort)
	if err != nil {
		return nil, err
	}
	return []tool.BaseTool{checkTool, killTool}, nil
}

// pidsByPort returns PIDs listening on the given port using OS-native commands.
func pidsByPort(port int) ([]int, string) {
	portStr := strconv.Itoa(port)
	var cmd *exec.Cmd
	if runtime.GOOS == "windows" {
		cmd = exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command",
			fmt.Sprintf(`Get-NetTCPConnection -LocalPort %s -State Listen -ErrorAction SilentlyContinue | Select-Object -ExpandProperty OwningProcess`, portStr))
	} else {
		cmd = exec.Command("sh", "-c", fmt.Sprintf("lsof -ti tcp:%s 2>/dev/null || ss -tlnp 2>/dev/null | awk '/:%-5s /{print $NF}' | grep -oP 'pid=\\K[0-9]+'", portStr, portStr))
	}
	out, err := cmd.Output()
	if err != nil || strings.TrimSpace(string(out)) == "" {
		return nil, ""
	}
	var pids []int
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		pid, err := strconv.Atoi(line)
		if err == nil && pid > 0 {
			pids = append(pids, pid)
		}
	}
	return pids, strings.TrimSpace(string(out))
}

// killPID sends SIGTERM (or SIGKILL if force) to the given PID.
func killPID(pid int, force bool) error {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	if runtime.GOOS == "windows" {
		// On Windows os.Process.Kill() sends TerminateProcess which is always forceful.
		return proc.Kill()
	}
	if force {
		return proc.Kill()
	}
	// SIGTERM
	return proc.Signal(os.Interrupt)
}
