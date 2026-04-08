package core

import (
	"build-agent/internal/agents"
	"build-agent/internal/config"

	"github.com/cloudwego/eino/adk"
)

type EventLog struct {
	AgentName string `json:"agent_name"`
	Role      string `json:"role,omitempty"`
	ToolName  string `json:"tool_name,omitempty"`
	Output    string `json:"output,omitempty"`
	Error     string `json:"error,omitempty"`
}

type RunResult struct {
	Output      string     `json:"output"`
	Events      []EventLog `json:"events,omitempty"`
	HasError    bool       `json:"has_error"`
	EvaluatedAt string     `json:"evaluated_at,omitempty"`
}

type Service struct {
	cfg    *config.Config
	agent  agents.Agent
	runner *adk.Runner
	// notifyRef 指向 newPlanExecuteService 里的 notifyHolder 变量，
	// 每次 RunTaskWithProgress 时更新它，使压缩通知路由到当前 onProgress。
	notifyRef *ProgressFunc
}

type ProgressFunc func(EventLog)
