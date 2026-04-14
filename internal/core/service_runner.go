package core

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"build-agent/internal/agents"
	"build-agent/internal/applog"

	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"
)

func (s *Service) RunTask(ctx context.Context, task string) (*RunResult, error) {
	return s.RunTaskWithProgress(ctx, task, nil)
}

func (s *Service) RunTaskWithProgress(ctx context.Context, task string, onProgress ProgressFunc) (*RunResult, error) {
	agentName := s.agent.Name()
	applog.AI("task.start", map[string]any{"agent": agentName, "task": task})

	normalized, err := s.agent.Workflow().NormalizeInput(task)
	if err != nil {
		applog.AI("task.error", map[string]any{"agent": agentName, "error": err.Error()})
		return nil, err
	}

	// Parallel analysis uses its own three-phase pipeline instead of a runner.
	if agentName == "analysis" && s.runner == nil {
		defer s.cleanupTemporaryArtifacts(onProgress)
		result, err := RunParallelAnalysis(ctx, s.cfg, normalized, onProgress)
		if err != nil {
			applog.AI("task.error", map[string]any{"agent": agentName, "error": err.Error()})
			return nil, err
		}
		applog.AI("task.done", map[string]any{"agent": agentName, "has_error": result.HasError, "output": result.Output})
		return result, nil
	}

	defer s.cleanupTemporaryArtifacts(onProgress)

	// 包装 onProgress 以记录每个事件
	wrappedProgress := func(ev EventLog) {
		applog.AI("event", map[string]any{
			"agent":    agentName,
			"ev_agent": ev.AgentName,
			"role":     ev.Role,
			"tool":     ev.ToolName,
			"output":   ev.Output,
			"error":    ev.Error,
		})
		if onProgress != nil {
			onProgress(ev)
		}
	}

	// 将压缩通知路由到当前 onProgress
	if s.notifyRef != nil {
		*s.notifyRef = wrappedProgress
		defer func() { *s.notifyRef = nil }()
	}

	output, events, hasError := s.runOnce(ctx, s.agent.Workflow().BuildTaskEnvelope(normalized), wrappedProgress)
	evaluatedAt := ""
	if pp := s.agent.PostProcessor(); pp != nil {
		post := pp.Process(agents.PostProcessInput{
			Output:   output,
			Events:   toPostProcessEvents(events),
			HasError: hasError,
		})
		output = post.Output
		hasError = post.HasError
		evaluatedAt = post.EvaluatedAt
		if onProgress != nil {
			for _, ev := range post.Progress {
				wrappedProgress(EventLog{
					AgentName: ev.AgentName,
					Role:      ev.Role,
					ToolName:  ev.ToolName,
					Output:    ev.Output,
					Error:     ev.Error,
				})
			}
		}
	}
	result := &RunResult{
		Output:      s.agent.Workflow().BuildFinalSummary(output),
		Events:      events,
		HasError:    hasError,
		EvaluatedAt: evaluatedAt,
	}
	applog.AI("task.done", map[string]any{"agent": agentName, "has_error": hasError, "output": result.Output})
	return result, nil
}

func toPostProcessEvents(events []EventLog) []agents.PostProcessEvent {
	out := make([]agents.PostProcessEvent, 0, len(events))
	for _, ev := range events {
		out = append(out, agents.PostProcessEvent{
			AgentName: ev.AgentName,
			Role:      ev.Role,
			ToolName:  ev.ToolName,
			Output:    ev.Output,
			Error:     ev.Error,
		})
	}
	return out
}

func (s *Service) runOnce(ctx context.Context, task string, onProgress ProgressFunc) (string, []EventLog, bool) {
	iter := s.runner.Query(ctx, task)
	events := make([]EventLog, 0, 8)
	finalOutput := ""
	hasError := false
	toolCalled := false
	scCfg := s.cfg.Agent[s.agent.Name()]
	agentName := s.agent.Name()
	execIter, planIter := 0, 0
	lastExec, lastPlan := -1, -1
	emitProgress := func() {
		if execIter == lastExec && planIter == lastPlan {
			return
		}
		lastExec, lastPlan = execIter, planIter
		item := EventLog{AgentName: "system", Output: fmt.Sprintf("iter exec=%d/%d plan=%d/%d", execIter, scCfg.ExecutorMaxIterations, planIter, scCfg.PlanExecuteMaxIterations)}
		events = append(events, item)
		if onProgress != nil {
			onProgress(item)
		}
	}
	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev == nil {
			continue
		}
		item := EventLog{AgentName: ev.AgentName}
		if ev.Err != nil {
			item.Error = ev.Err.Error()
			// Only mark hasError for the top-level agent's own errors, not sub-agent errors
			// that are already handled and returned as tool results.
			if isOwnEvent(ev, agentName) {
				hasError = true
			}
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			item.Role = string(ev.Output.MessageOutput.Role)
			item.ToolName = ev.Output.MessageOutput.ToolName
		}
		if txt := extractEventText(ev); txt != "" {
			item.Output = txt
			// Only update finalOutput from the top-level agent's own text events.
			if isOwnEvent(ev, agentName) {
				finalOutput = txt
			}
		}
		// Only count tool/plan iterations for the top-level agent's own events.
		if isOwnEvent(ev, agentName) {
			if isToolEvent(ev) {
				toolCalled = true
				execIter++
				emitProgress()
				if runErr := RunCommandErrorFromEvent(item); runErr != "" && item.Error == "" {
					if item.Output != "" {
						item.Output = strings.TrimSpace(item.Output) + "\n[warn] " + runErr
					} else {
						item.Output = "[warn] " + runErr
					}
				}
			} else if isPlanIterationEvent(ev, item) {
				planIter++
				emitProgress()
			}
		}
		if item.Output != "" || item.Error != "" {
			events = append(events, item)
			if onProgress != nil {
				onProgress(item)
			}
		}
	}
	if strings.TrimSpace(finalOutput) == "" && len(events) > 0 {
		finalOutput = events[len(events)-1].Output
	}
	if !toolCalled {
		hasError = true
		if strings.TrimSpace(finalOutput) == "" {
			finalOutput = "未检测到任何工具调用，任务未实际执行。"
		}
	}
	return finalOutput, events, hasError
}

// isOwnEvent returns true if the event belongs to the top-level agent (not a sub-agent).
// Sub-agent events emitted via EmitInternalEvents have a different AgentName.
func isOwnEvent(ev *adk.AgentEvent, agentName string) bool {
	if ev.AgentName == "" {
		return true // untagged events belong to the top-level agent
	}
	evName := strings.ToLower(strings.TrimSpace(ev.AgentName))
	own := strings.ToLower(strings.TrimSpace(agentName))
	// planexecute internal agents are named like "planner", "executor", "replanner"
	// which don't match sub-agent names like "code", "eval", "analysis".
	if evName == own {
		return true
	}
	// Events from planexecute internals (planner/executor/replanner) belong to the top-level agent.
	if strings.Contains(evName, "planner") || strings.Contains(evName, "executor") ||
		strings.Contains(evName, "replanner") || strings.Contains(evName, "planexecute") {
		return true
	}
	return false
}

func extractEventText(ev *adk.AgentEvent) string {
	if ev.Output == nil {
		return ""
	}
	if resp, ok := ev.Output.CustomizedOutput.(*planexecute.Response); ok && resp != nil {
		return strings.TrimSpace(resp.Response)
	}
	if resp, ok := ev.Output.CustomizedOutput.(planexecute.Response); ok {
		return strings.TrimSpace(resp.Response)
	}
	if ev.Output.MessageOutput != nil {
		msg, err := ev.Output.MessageOutput.GetMessage()
		if err == nil && msg != nil {
			return unwrapJSONText(strings.TrimSpace(msg.Content))
		}
	}
	return ""
}

// unwrapJSONText 尝试从 {"response":"..."} 等 JSON 包装中提取纯文本
func unwrapJSONText(s string) string {
	if !strings.HasPrefix(s, "{") {
		return s
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &obj); err != nil {
		return s
	}
	for _, key := range []string{"response", "content", "output", "text", "result"} {
		if v, ok := obj[key]; ok {
			var text string
			if err := json.Unmarshal(v, &text); err == nil && strings.TrimSpace(text) != "" {
				return strings.TrimSpace(text)
			}
		}
	}
	return s
}

func isToolEvent(ev *adk.AgentEvent) bool {
	return ev != nil && ev.Output != nil && ev.Output.MessageOutput != nil && ev.Output.MessageOutput.Role == schema.Tool
}

func isPlanIterationEvent(ev *adk.AgentEvent, logItem EventLog) bool {
	if ev == nil || isToolEvent(ev) {
		return false
	}
	name := strings.ToLower(strings.TrimSpace(ev.AgentName))
	if strings.Contains(name, "planner") || strings.Contains(name, "replanner") || strings.Contains(name, "planexecute") {
		return true
	}
	return strings.Contains(strings.TrimSpace(logItem.Output), "\"steps\"")
}

func (s *Service) cleanupTemporaryArtifacts(onProgress ProgressFunc) {
	tempDir := s.agent.ToolPolicy().TempDirName
	if tempDir == "" {
		tempDir = ".build-agent-tmp"
	}
	target := filepath.Join(s.cfg.Base.WorkspaceRoot, tempDir)
	info, err := os.Stat(target)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return
		}
		if onProgress != nil {
			onProgress(EventLog{AgentName: "system", Error: fmt.Sprintf("临时目录状态检查失败: %v", err)})
		}
		return
	}
	if !info.IsDir() {
		if onProgress != nil {
			onProgress(EventLog{AgentName: "system", Error: fmt.Sprintf("临时路径不是目录，跳过清理: %s", target)})
		}
		return
	}
	const attempts = 5
	const delay = 200 * time.Millisecond
	var lastErr error
	for i := 0; i < attempts; i++ {
		if i > 0 {
			time.Sleep(delay)
		}
		lastErr = os.RemoveAll(target)
		if lastErr == nil {
			return
		}
	}
	if lastErr != nil && onProgress != nil {
		onProgress(EventLog{AgentName: "system", Error: fmt.Sprintf("临时目录清理失败（已重试 %d 次）: %v", attempts, lastErr)})
	}
}

func RunCommandErrorFromEvent(ev EventLog) string {
	if ev.ToolName != "run_command" || strings.TrimSpace(ev.Output) == "" {
		return ""
	}
	var obj map[string]any
	if err := json.Unmarshal([]byte(ev.Output), &obj); err != nil {
		return ""
	}
	code, _ := obj["exit_code"].(float64)
	if int(code) == 0 {
		return ""
	}
	return fmt.Sprintf("run_command exit_code=%d", int(code))
}
