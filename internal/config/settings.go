package config

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// Settings 是持久化到磁盘的用户配置（JSON 格式）
type Settings struct {
	Model  ModelSettings            `json:"model"`
	Agents map[string]AgentSettings `json:"agents"`
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
