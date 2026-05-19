package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Settings 是持久化到磁盘的用户配置（JSON 格式）
type Settings struct {
	Model      ModelSettings            `json:"model"`
	Agents     map[string]AgentSettings `json:"agents"`
	MCPServers json.RawMessage          `json:"mcpServers,omitempty"`
}

// MCPServerConfig 描述通过 stdio 运行的 MCP 服务器子进程（command + args；可选 env、cwd、disabled；超时字段为本应用扩展）。
type MCPServerConfig struct {
	Disabled        bool              `json:"disabled"`
	Command         string            `json:"command"`
	Args            []string          `json:"args"`
	Env             map[string]string `json:"env"`
	Cwd             string            `json:"cwd"`
	StartTimeoutSec int               `json:"startTimeoutSec"`
	CallTimeoutSec  int               `json:"callTimeoutSec"`
}

// ModelSettings 模型配置（OpenAI 标准）
type ModelSettings struct {
	BaseURL                string `json:"baseUrl"`
	APIKey                 string `json:"apiKey"`
	Model                  string `json:"model"`
	MaxContextTokens       int    `json:"maxContextTokens"`
	SmartCompressThreshold int    `json:"smartCompressThreshold"`
}

// AgentSettings 单个智能体配置
type AgentSettings struct {
	ExecutorMaxIterations    int `json:"executorMaxIterations"`
	PlanExecuteMaxIterations int `json:"planExecuteMaxIterations"`
}

var defaultSettings = Settings{
	Model: ModelSettings{
		BaseURL:                "https://api.openai.com/v1",
		APIKey:                 "",
		Model:                  "gpt-4o-mini",
		MaxContextTokens:       130000,
		SmartCompressThreshold: 100000,
	},
	Agents: map[string]AgentSettings{
		"analysis":     {ExecutorMaxIterations: 100, PlanExecuteMaxIterations: 5},
		"requirements": {ExecutorMaxIterations: 100, PlanExecuteMaxIterations: 100},
		"code":         {ExecutorMaxIterations: 1000, PlanExecuteMaxIterations: 10},
		"eval":         {ExecutorMaxIterations: 1000, PlanExecuteMaxIterations: 10},
		"build":        {ExecutorMaxIterations: 1000, PlanExecuteMaxIterations: 10},
		"chat":         {ExecutorMaxIterations: 200, PlanExecuteMaxIterations: 10},
	},
	MCPServers: json.RawMessage([]byte("{}")),
}

// SettingsPath 返回配置文件路径
func SettingsPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	dir := filepath.Join(base, "build-agent")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "settings.json")
}

// LoadSettings 从磁盘加载配置，文件不存在时返回默认值
func LoadSettings() (*Settings, error) {
	data, err := os.ReadFile(SettingsPath())
	if err != nil {
		if os.IsNotExist(err) {
			s := defaultSettings
			return &s, nil
		}
		return nil, err
	}
	// 从默认值开始，再用文件内容覆盖（保证新增字段有默认值）
	s := defaultSettings
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	// 补全缺失的 agent 配置
	for name, def := range defaultSettings.Agents {
		if _, ok := s.Agents[name]; !ok {
			s.Agents[name] = def
		}
	}
	if len(s.MCPServers) == 0 {
		s.MCPServers = json.RawMessage([]byte("{}"))
	}
	return &s, nil
}

// SaveSettings 将配置写入磁盘
func SaveSettings(s *Settings) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	p := SettingsPath()
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, data, 0644); err != nil {
		return err
	}
	if err := os.Rename(tmp, p); err != nil {
		os.Remove(tmp)
		return err
	}
	return nil
}

// SettingsFromSaveRequest 根据前端提交的字段构造 Settings；校验 MCP JSON 但不改写其字节内容。
func SettingsFromSaveRequest(model ModelSettings, agents map[string]AgentSettings, mcpJSON string) (*Settings, error) {
	raw := mcpJSON
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	} else if !json.Valid([]byte(raw)) {
		t := strings.TrimSpace(raw)
		if !json.Valid([]byte(t)) {
			return nil, fmt.Errorf("MCP 配置不是合法的 JSON")
		}
		raw = t
	}
	if _, err := MCPServersMap([]byte(raw)); err != nil {
		return nil, err
	}
	out := Settings{
		Model:      model,
		Agents:     make(map[string]AgentSettings),
		MCPServers: json.RawMessage(raw),
	}
	for k, v := range defaultSettings.Agents {
		out.Agents[k] = v
	}
	if agents != nil {
		for k, v := range agents {
			out.Agents[k] = v
		}
	}
	return &out, nil
}

// LastWorkspace 返回最近打开的工作区路径，没有记录时返回空字符串
func LastWorkspace() string {
	data, err := os.ReadFile(workspacesPath())
	if err != nil {
		return ""
	}
	var h struct {
		Recent []struct {
			Path string `json:"path"`
		} `json:"recent"`
	}
	if err := json.Unmarshal(data, &h); err != nil || len(h.Recent) == 0 {
		return ""
	}
	return h.Recent[0].Path
}

func workspacesPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	dir := filepath.Join(base, "build-agent")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "workspaces.json")
}
