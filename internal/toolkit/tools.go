package toolkit

import (
	"time"

	"github.com/cloudwego/eino/components/tool"
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
