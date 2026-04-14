package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"build-agent/internal/agents"
	"build-agent/internal/config"
	"build-agent/internal/toolkit"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	"github.com/cloudwego/eino/schema"
)

// parallelAnalysisMaxConcurrency 模块 agent 的最大并发数。
const parallelAnalysisMaxConcurrency = 20

// moduleTask 描述一个需要并行分析的模块。
type moduleTask struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	Type        string `json:"type"`
	Description string `json:"description"`
}

// plannerOutput 是规划器 agent 输出的 JSON 结构。
type plannerOutput struct {
	ProjectName string       `json:"project_name"`
	Modules     []moduleTask `json:"modules"`
}

// moduleResult 保存单个模块 agent 产出的分析文本。
type moduleResult struct {
	Task    moduleTask
	Output  string
	Elapsed time.Duration
	Err     error
}

// RunParallelAnalysis 执行三阶段并行分析：
//  1. 规划阶段 — 扫描工作区，输出 JSON 模块列表
//  2. 并行阶段 — 每个模块启动一个独立的 planexecute agent 并发执行，结果写入临时文件
//  3. 汇总阶段 — 从临时文件读取各模块报告，合并生成 design.md，最后清理临时目录
func RunParallelAnalysis(ctx context.Context, cfg *config.Config, task string, onProgress ProgressFunc) (*RunResult, error) {
	emit := func(msg string) {
		if onProgress != nil {
			onProgress(EventLog{AgentName: "analysis-parallel", Output: msg})
		}
	}

	// 临时目录固定在工作区下的 .analysis-agent-tmp。
	tmpDir := filepath.Join(cfg.Base.WorkspaceRoot, ".analysis-agent-tmp")
	if err := os.MkdirAll(tmpDir, 0o755); err != nil {
		return &RunResult{Output: fmt.Sprintf("创建临时目录失败: %v", err), HasError: true}, nil
	}
	// 任务结束时清理临时目录（无论成功或失败）。
	defer func() {
		if err := os.RemoveAll(tmpDir); err != nil {
			emit(fmt.Sprintf("⚠️  临时目录清理失败: %v", err))
		}
	}()

	// ── 阶段1：规划 ──────────────────────────────────────────────────────────
	emit("🗂️  阶段1/3：扫描项目结构，识别模块列表…")
	plan, err := runPlannerPhase(ctx, cfg, task, onProgress)
	if err != nil {
		return &RunResult{Output: fmt.Sprintf("规划阶段失败: %v", err), HasError: true}, nil
	}
	emit(fmt.Sprintf("✅  识别到 %d 个模块：%s", len(plan.Modules), moduleNames(plan.Modules)))

	// ── 阶段2：并行模块分析 ──────────────────────────────────────────────────
	emit(fmt.Sprintf("⚡  阶段2/3：并行分析各模块（最大并发 %d）…", parallelAnalysisMaxConcurrency))
	results := runModulesParallel(ctx, cfg, plan, tmpDir, onProgress)

	// 上报各模块执行结果
	for _, r := range results {
		if r.Err != nil {
			emit(fmt.Sprintf("⚠️  [%s] 分析失败（%v），将在汇总时标注", r.Task.Name, r.Err))
		} else {
			emit(fmt.Sprintf("✅  [%s] 分析完成（耗时 %s，已写入临时文件）", r.Task.Name, r.Elapsed.Round(time.Second)))
		}
	}

	// ── 阶段3：汇总 ──────────────────────────────────────────────────────────
	emit("📝  阶段3/3：从临时文件读取各模块报告，生成 design.md…")
	result, err := runSynthesisPhase(ctx, cfg, plan, results, tmpDir, task, onProgress)
	if err != nil {
		return &RunResult{Output: fmt.Sprintf("汇总阶段失败: %v", err), HasError: true}, nil
	}
	return result, nil
}

// ── 阶段1 实现 ───────────────────────────────────────────────────────────────

// runPlannerPhase 分两步执行：
//  1. 直接读取工作区根目录列表（不经过 LLM）
//  2. 将目录列表交给 LLM 做一次性分类，输出 JSON 模块列表
//
// 不使用 planexecute，避免 agent 自主把整个分析任务做完后输出自然语言总结。
func runPlannerPhase(ctx context.Context, cfg *config.Config, _ string, onProgress ProgressFunc) (*plannerOutput, error) {
	// 步骤1：直接读取根目录，过滤掉无关条目。
	entries, err := scanWorkspaceRoot(cfg.Base.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("扫描工作区根目录失败: %w", err)
	}
	if onProgress != nil {
		onProgress(EventLog{AgentName: "analysis-planner", Output: fmt.Sprintf("扫描到 %d 个子目录", len(entries))})
	}

	// 步骤2：将目录列表交给 LLM，一次性输出 JSON。
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Model.APIKey,
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("创建规划器模型失败: %w", err)
	}

	userMsg := fmt.Sprintf(
		"工作区根目录：%s\n\n以下是根目录下的子目录列表：\n%s\n\n请按照系统提示的格式输出 JSON，不要输出任何其他内容。",
		cfg.Base.WorkspaceRoot,
		strings.Join(entries, "\n"),
	)

	systemPrompt := agents.BuildParallelAnalysisPlannerPrompt(cfg.Base.WorkspaceRoot)
	msgs := []*schema.Message{
		schema.SystemMessage(systemPrompt),
		schema.UserMessage(userMsg),
	}

	resp, err := model.Generate(ctx, msgs)
	if err != nil {
		return nil, fmt.Errorf("规划器 LLM 调用失败: %w", err)
	}

	output := strings.TrimSpace(resp.Content)
	plan, err := parsePlannerOutput(output)
	if err != nil {
		return nil, fmt.Errorf("解析规划器 JSON 失败: %w\n原始输出:\n%s", err, output)
	}
	if len(plan.Modules) == 0 {
		return nil, fmt.Errorf("规划器返回了空的模块列表")
	}
	return plan, nil
}

// skipDirs 是扫描根目录时需要忽略的目录名。
var skipDirs = map[string]bool{
	".git": true, ".idea": true, ".vscode": true, ".mvn": true,
	"node_modules": true, "target": true, "dist": true, "build": true,
	".spec": true, "checkstyle": true,
}

// scanWorkspaceRoot 读取工作区根目录，返回所有子目录名（已过滤构建/IDE 目录）。
func scanWorkspaceRoot(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}
	var dirs []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if skipDirs[name] || strings.HasPrefix(name, ".") {
			continue
		}
		dirs = append(dirs, filepath.ToSlash(name))
	}
	return dirs, nil
}

// parsePlannerOutput 从规划器的输出中提取 JSON 对象。
// 模型可能会用 markdown 代码块包裹 JSON，因此先剥离代码块标记。
func parsePlannerOutput(raw string) (*plannerOutput, error) {
	s := strings.TrimSpace(raw)
	// 剥离 ```json ... ``` 或 ``` ... ``` 代码块
	if idx := strings.Index(s, "```"); idx != -1 {
		s = s[idx:]
		s = strings.TrimPrefix(s, "```json")
		s = strings.TrimPrefix(s, "```")
		if end := strings.LastIndex(s, "```"); end != -1 {
			s = s[:end]
		}
		s = strings.TrimSpace(s)
	}
	// 定位最外层 JSON 对象
	start := strings.Index(s, "{")
	end := strings.LastIndex(s, "}")
	if start == -1 || end == -1 || end <= start {
		return nil, fmt.Errorf("未找到 JSON 对象")
	}
	s = s[start : end+1]

	var out plannerOutput
	if err := json.Unmarshal([]byte(s), &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// ── 阶段2 实现 ───────────────────────────────────────────────────────────────

func runModulesParallel(ctx context.Context, cfg *config.Config, plan *plannerOutput, tmpDir string, onProgress ProgressFunc) []moduleResult {
	results := make([]moduleResult, len(plan.Modules))
	sem := make(chan struct{}, parallelAnalysisMaxConcurrency)
	var wg sync.WaitGroup

	for i, mod := range plan.Modules {
		wg.Add(1)
		go func(idx int, m moduleTask) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			start := time.Now()
			output, err := runSingleModuleAgent(ctx, cfg, m, tmpDir, onProgress)
			results[idx] = moduleResult{
				Task:    m,
				Output:  output,
				Elapsed: time.Since(start),
				Err:     err,
			}
		}(i, mod)
	}

	wg.Wait()
	return results
}

func runSingleModuleAgent(ctx context.Context, cfg *config.Config, m moduleTask, tmpDir string, onProgress ProgressFunc) (string, error) {
	scCfg := cfg.Agent["analysis"]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Model.APIKey,
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
	})
	if err != nil {
		return "", fmt.Errorf("为模块 %s 创建模型失败: %w", m.Name, err)
	}

	// 模块临时文件路径，用于写入分析结果。
	tmpFilePath := filepath.ToSlash(filepath.Join(tmpDir, m.Name+".md"))

	tools, err := toolkit.BuildTools(cfg.Base.WorkspaceRoot, cfg.Base.CmdTimeoutSec, toolkit.Policy{
		AllowRunCommand:       false,
		MissingPathAsExistsNo: true,
		// 允许写入临时目录，禁止写入其他位置。
		WriteAllowPrefixes: []string{".analysis-agent-tmp"},
	})
	if err != nil {
		return "", err
	}

	systemPrompt := agents.BuildModuleAnalysisPrompt(cfg.Base.WorkspaceRoot, m.Path, m.Name, m.Type, tmpFilePath)
	pb := agents.PromptBuilder{
		Planner:  func() string { return systemPrompt },
		Executor: func() string { return systemPrompt },
		Replanner: func() string {
			return "若已完成模块分析并将 Markdown 写入临时文件，直接返回完成。否则继续分析。"
		},
	}

	agentInst, err := buildPlanExecuteAgent(ctx, model, pb, tools, scCfg,
		cfg.Model.MaxContextTokens, cfg.Model.SmartCompressThreshold, nil)
	if err != nil {
		return "", err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true})
	task := fmt.Sprintf("WORKSPACE_ROOT=%s\nMODULE=%s\nMODULE_PATH=%s\nTMP_FILE=%s\nTASK:\n分析模块 %s，将结果写入 TMP_FILE",
		cfg.Base.WorkspaceRoot, m.Name, m.Path, tmpFilePath, m.Name)

	// 进度事件加上模块名前缀，方便 UI 区分来源。
	prefixedProgress := func(ev EventLog) {
		ev.AgentName = fmt.Sprintf("module/%s", m.Name)
		if onProgress != nil {
			onProgress(ev)
		}
	}

	_, _, hasError := runnerCollect(ctx, runner, task, fmt.Sprintf("module/%s", m.Name), prefixedProgress)

	// 无论 agent 是否报错，都尝试从临时文件读取结果。
	// agent 可能已经写入了部分内容，即使最终报错也应保留。
	content, readErr := os.ReadFile(tmpFilePath)
	if readErr != nil {
		if hasError {
			return "", fmt.Errorf("模块 agent 执行出错且临时文件不存在")
		}
		// agent 没报错但没写文件，说明 agent 只返回了文本而没有调用 write_file。
		// 这种情况不应发生（prompt 要求写文件），记录警告但不失败。
		return "", fmt.Errorf("模块 agent 未写入临时文件 %s", tmpFilePath)
	}

	output := strings.TrimSpace(string(content))
	if hasError && output == "" {
		return "", fmt.Errorf("模块 agent 执行出错")
	}
	return output, nil
}

// ── 阶段3 实现 ───────────────────────────────────────────────────────────────

func runSynthesisPhase(ctx context.Context, cfg *config.Config, plan *plannerOutput, results []moduleResult, tmpDir string, originalTask string, onProgress ProgressFunc) (*RunResult, error) {
	scCfg := cfg.Agent["analysis"]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Model.APIKey,
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("创建汇总模型失败: %w", err)
	}

	// 汇总阶段需要写权限以生成 design.md。
	tools, err := toolkit.BuildTools(cfg.Base.WorkspaceRoot, cfg.Base.CmdTimeoutSec, toolkit.Policy{
		AllowRunCommand:       false,
		MissingPathAsExistsNo: true,
	})
	if err != nil {
		return nil, err
	}

	designSpecRel := cfg.Agent["analysis"].DesignSpecRel
	systemPrompt := agents.BuildSynthesisPrompt(cfg.Base.WorkspaceRoot, designSpecRel)
	pb := agents.PromptBuilder{
		Planner:  func() string { return systemPrompt },
		Executor: func() string { return systemPrompt },
		Replanner: func() string {
			return "若 design.md 已成功写入并通过 read_file 校验，直接返回完成。否则继续。"
		},
	}

	agentInst, err := buildPlanExecuteAgent(ctx, model, pb, tools, scCfg,
		cfg.Model.MaxContextTokens, cfg.Model.SmartCompressThreshold, nil)
	if err != nil {
		return nil, err
	}

	runner := adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true})

	// 构建任务信封：优先从临时文件读取模块报告，回退到内存中的 output。
	var sb strings.Builder
	fmt.Fprintf(&sb, "WORKSPACE_ROOT=%s\n", cfg.Base.WorkspaceRoot)
	fmt.Fprintf(&sb, "DESIGN_SPEC_PATH=%s\n", designSpecRel)
	fmt.Fprintf(&sb, "PROJECT_NAME=%s\n", plan.ProjectName)
	fmt.Fprintf(&sb, "ORIGINAL_TASK:\n%s\n\n", originalTask)
	sb.WriteString("## 各模块分析报告\n\n")

	for _, r := range results {
		fmt.Fprintf(&sb, "### 模块：%s（%s，路径：%s）\n\n", r.Task.Name, r.Task.Type, r.Task.Path)
		if r.Err != nil {
			fmt.Fprintf(&sb, "**分析失败：%v**\n\n", r.Err)
		} else {
			// 优先从临时文件读取，保证内容与磁盘一致。
			tmpFile := filepath.Join(tmpDir, r.Task.Name+".md")
			if fileContent, readErr := os.ReadFile(tmpFile); readErr == nil {
				sb.WriteString(strings.TrimSpace(string(fileContent)))
			} else {
				// 临时文件不存在时回退到内存结果。
				sb.WriteString(r.Output)
			}
			sb.WriteString("\n\n")
		}
		sb.WriteString("---\n\n")
	}

	sb.WriteString("TASK:\n整合以上各模块报告，生成完整的项目设计文档并写入 design.md。")

	output, events, hasError := runnerCollect(ctx, runner, sb.String(), "analysis-synthesis", onProgress)

	return &RunResult{
		Output:   output,
		Events:   events,
		HasError: hasError,
	}, nil
}

// ── 公共辅助函数 ──────────────────────────────────────────────────────────────

// runnerCollect 消费 adk.Runner 的迭代器，返回最终文本输出、所有 EventLog 条目以及是否发生错误。
func runnerCollect(ctx context.Context, runner *adk.Runner, task string, agentLabel string, onProgress ProgressFunc) (string, []EventLog, bool) {
	iter := runner.Query(ctx, task)
	events := make([]EventLog, 0, 8)
	finalOutput := ""
	hasError := false

	for {
		ev, ok := iter.Next()
		if !ok {
			break
		}
		if ev == nil {
			continue
		}

		item := EventLog{AgentName: agentLabel}

		if ev.Err != nil {
			item.Error = ev.Err.Error()
			hasError = true
		}
		if ev.Output != nil && ev.Output.MessageOutput != nil {
			item.Role = string(ev.Output.MessageOutput.Role)
			item.ToolName = ev.Output.MessageOutput.ToolName
		}

		if txt := extractEventTextFromEvent(ev); txt != "" {
			item.Output = txt
			finalOutput = txt
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
	return finalOutput, events, hasError
}

// extractEventTextFromEvent 与 service_runner.go 中的同名逻辑保持一致，
// 独立实现以避免包内循环引用。
func extractEventTextFromEvent(ev *adk.AgentEvent) string {
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
			return strings.TrimSpace(msg.Content)
		}
	}
	return ""
}

func moduleNames(mods []moduleTask) string {
	names := make([]string, len(mods))
	for i, m := range mods {
		names[i] = m.Name
	}
	return strings.Join(names, ", ")
}
