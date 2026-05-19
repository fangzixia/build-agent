package config

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
)

// MCPServersMap 从用户保存的 MCP 配置 JSON 解析运行时映射（支持顶层仅含 mcpServers 的包装对象）。
func MCPServersMap(raw []byte) (map[string]MCPServerConfig, error) {
	raw = bytes.TrimSpace(raw)
	if len(raw) == 0 {
		return map[string]MCPServerConfig{}, nil
	}
	if !json.Valid(raw) {
		return nil, fmt.Errorf("MCP 配置不是合法的 JSON")
	}
	var top map[string]json.RawMessage
	if err := json.Unmarshal(raw, &top); err != nil {
		return nil, fmt.Errorf("解析 MCP JSON: %w", err)
	}
	if len(top) == 0 {
		return map[string]MCPServerConfig{}, nil
	}
	payload := raw
	if inner, ok := top["mcpServers"]; ok && len(top) == 1 {
		payload = inner
	}
	var m map[string]MCPServerConfig
	if err := json.Unmarshal(payload, &m); err != nil {
		return nil, fmt.Errorf("解析 MCP 服务器列表: %w", err)
	}
	for name, cfg := range m {
		if strings.TrimSpace(name) == "" {
			return nil, fmt.Errorf("存在空的服务器名称键")
		}
		if !cfg.Disabled && strings.TrimSpace(cfg.Command) == "" {
			return nil, fmt.Errorf("服务器 %q 未填写 command", name)
		}
	}
	return m, nil
}
