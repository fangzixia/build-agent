package mcpclient

import (
	"context"
	"sort"
	"strings"

	"build-agent/internal/config"
)

// MCPToolSummary 探测到的单个工具摘要（用于 UI）
type MCPToolSummary struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// MCPServerProbeResult 单个 MCP 服务器的探测结果
type MCPServerProbeResult struct {
	Name       string           `json:"name"`
	Skipped    bool             `json:"skipped"`
	SkipReason string           `json:"skipReason,omitempty"`
	OK         bool             `json:"ok"`
	Error      string           `json:"error,omitempty"`
	Tools      []MCPToolSummary `json:"tools,omitempty"`
}

// ProbeServers 依次连接各 MCP 并列出 tools/list；单个失败不影响其它服务器。
func ProbeServers(ctx context.Context, servers map[string]config.MCPServerConfig, workspaceRoot string, defaultCallTimeoutSec int) []MCPServerProbeResult {
	if len(servers) == 0 {
		return nil
	}
	names := make([]string, 0, len(servers))
	for name := range servers {
		names = append(names, name)
	}
	sort.Strings(names)

	out := make([]MCPServerProbeResult, 0, len(names))
	for _, serverKey := range names {
		sc := servers[serverKey]
		res := MCPServerProbeResult{Name: serverKey}
		if sc.Disabled {
			res.Skipped = true
			res.SkipReason = "disabled"
			out = append(out, res)
			continue
		}
		if strings.TrimSpace(sc.Command) == "" {
			res.Skipped = true
			res.SkipReason = "empty command"
			out = append(out, res)
			continue
		}

		sess, closer, err := connectServer(ctx, serverKey, sc, workspaceRoot)
		if err != nil {
			res.OK = false
			res.Error = err.Error()
			out = append(out, res)
			continue
		}

		listTO := callTimeoutDuration(sc.CallTimeoutSec, defaultCallTimeoutSec)
		listCtx := ctx
		var listCancel context.CancelFunc
		if listTO > 0 {
			listCtx, listCancel = context.WithTimeout(ctx, listTO)
		}
		toolsRes, err := sess.ListTools(listCtx, nil)
		if listCancel != nil {
			listCancel()
		}
		closer()

		if err != nil {
			res.OK = false
			res.Error = err.Error()
			out = append(out, res)
			continue
		}

		res.OK = true
		for _, tl := range toolsRes.Tools {
			if tl == nil || tl.Name == "" {
				continue
			}
			res.Tools = append(res.Tools, MCPToolSummary{
				Name:        tl.Name,
				Description: strings.TrimSpace(tl.Description),
			})
		}
		out = append(out, res)
	}
	return out
}
