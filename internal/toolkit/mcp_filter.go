package toolkit

import (
	"strings"

	"build-agent/internal/config"
)

// filterMCPServersByAllowlist 按 MCP 服务器键名过滤；allowlist 非 nil 且为空时返回空映射。
func filterMCPServersByAllowlist(all map[string]config.MCPServerConfig, allowlist *[]string) map[string]config.MCPServerConfig {
	if allowlist == nil {
		return all
	}
	if len(*allowlist) == 0 {
		return map[string]config.MCPServerConfig{}
	}
	out := make(map[string]config.MCPServerConfig)
	for _, k := range *allowlist {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if sc, ok := all[k]; ok {
			out[k] = sc
		}
	}
	return out
}
