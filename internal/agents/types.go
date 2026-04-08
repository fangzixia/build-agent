package agents

import (
	"encoding/json"
	"fmt"
	"strings"

	"build-agent/internal/toolkit"
)

type Workflow interface {
	NormalizeInput(task string) (string, error)
	BuildTaskEnvelope(task string) string
	BuildFinalSummary(raw string) string
}

type Agent interface {
	Name() string
	PromptBuilder() PromptBuilder
	Workflow() Workflow
	ToolPolicy() toolkit.Policy
	PostProcessor() PostProcessor
}

type PostProcessEvent struct {
	AgentName string
	Role      string
	ToolName  string
	Output    string
	Error     string
}

type PostProcessInput struct {
	Output   string
	Events   []PostProcessEvent
	HasError bool
}

type PostProcessResult struct {
	Output      string
	HasError    bool
	EvaluatedAt string
	Progress    []PostProcessEvent
}

type PostProcessor interface {
	Process(in PostProcessInput) PostProcessResult
}

type PromptBuilder struct {
	Planner   func() string
	Executor  func() string
	Replanner func() string
}

type basicWorkflow struct {
	baseTask      string
	envelopeLines []string
}

func (w basicWorkflow) NormalizeInput(task string) (string, error) {
	trimmed := strings.TrimSpace(task)
	if trimmed == "" {
		if w.baseTask == "" {
			return "", fmt.Errorf("task cannot be empty")
		}
		return w.baseTask, nil
	}
	return trimmed, nil
}

func (w basicWorkflow) BuildTaskEnvelope(task string) string {
	return strings.Join(append(w.envelopeLines, "TASK:\n"+task), "\n")
}

func (w basicWorkflow) BuildFinalSummary(raw string) string {
	out := strings.TrimSpace(raw)
	if out == "" {
		return "任务执行完成，但模型未返回文本输出。"
	}
	// planexecute 的 respond tool 有时返回 JSON 格式 {"response": "..."}，提取纯文本
	if strings.HasPrefix(out, "{") {
		var obj map[string]json.RawMessage
		if err := json.Unmarshal([]byte(out), &obj); err == nil {
			for _, key := range []string{"response", "content", "output", "text", "result"} {
				if v, ok := obj[key]; ok {
					var s string
					if err := json.Unmarshal(v, &s); err == nil && strings.TrimSpace(s) != "" {
						return strings.TrimSpace(s)
					}
				}
			}
		}
	}
	return out
}

type agentImpl struct {
	name          string
	prompt        PromptBuilder
	workflow      Workflow
	policy        toolkit.Policy
	postProcessor PostProcessor
}

func (s agentImpl) Name() string                 { return s.name }
func (s agentImpl) PromptBuilder() PromptBuilder { return s.prompt }
func (s agentImpl) Workflow() Workflow           { return s.workflow }
func (s agentImpl) ToolPolicy() toolkit.Policy   { return s.policy }
func (s agentImpl) PostProcessor() PostProcessor { return s.postProcessor }
