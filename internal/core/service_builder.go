package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"build-agent/internal/agents"
	"build-agent/internal/config"
	"build-agent/internal/toolkit"

	openai "github.com/cloudwego/eino-ext/components/model/openai"
	"github.com/cloudwego/eino/adk"
	"github.com/cloudwego/eino/adk/prebuilt/planexecute"
	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func NewService(ctx context.Context, cfg *config.Config, agentName string) (*Service, error) {
	switch agentName {
	case "build":
		return newBuildAgentService(ctx, cfg)
	case "analysis":
		return newParallelAnalysisService(ctx, cfg)
	default:
		return newPlanExecuteService(ctx, cfg, agentName)
	}
}

// newParallelAnalysisService 返回一个 Service，其 RunTaskWithProgress 会委托给
// 三阶段并行分析流水线，而不是单个 planexecute agent。
func newParallelAnalysisService(ctx context.Context, cfg *config.Config) (*Service, error) {
	agentDef, err := agents.Build("analysis", cfg)
	if err != nil {
		return nil, err
	}
	// runner 为 nil，RunTaskWithProgress 会检测到这一点并走并行分析路径。
	svc := &Service{
		cfg:   cfg,
		agent: agentDef,
	}
	return svc, nil
}

func newPlanExecuteService(ctx context.Context, cfg *config.Config, agentName string) (*Service, error) {
	scn, err := agents.Build(agentName, cfg)
	if err != nil {
		return nil, err
	}
	scCfg := cfg.Agent[agentName]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Model.APIKey,
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}
	tools, err := toolkit.BuildTools(cfg.Base.WorkspaceRoot, cfg.Base.CmdTimeoutSec, scn.ToolPolicy())
	if err != nil {
		return nil, err
	}
	pb := scn.PromptBuilder()

	// notifyHolder 是一个间接层：buildPlanExecuteAgent 在构建时不知道 onProgress，
	// 通过持有指针，在运行时动态路由压缩通知到当前的 onProgress 回调。
	var notifyHolder ProgressFunc
	notify := func(msg string) {
		if notifyHolder != nil {
			notifyHolder(EventLog{AgentName: "system", Output: msg})
		}
	}

	agentInst, err := buildPlanExecuteAgent(ctx, model, pb, tools, scCfg, cfg.Model.MaxContextTokens, cfg.Model.SmartCompressThreshold, notify)
	if err != nil {
		return nil, err
	}
	svc := &Service{
		cfg:    cfg,
		agent:  scn,
		runner: adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true}),
	}
	// 将 notifyHolder 指向 svc.onProgress，这样压缩通知会自动路由到当前执行的 onProgress
	svc.notifyRef = &notifyHolder
	return svc, nil
}

// newBuildAgentService 为 "build" agent 构建 planexecute Service，
// 其中 code/eval/analysis 子 agent 以工具形式暴露给 build executor。
func newBuildAgentService(ctx context.Context, cfg *config.Config) (*Service, error) {
	codeAgent, err := buildSubAgent(ctx, cfg, "code")
	if err != nil {
		return nil, fmt.Errorf("build code sub-agent: %w", err)
	}
	evalAgent, err := buildSubAgent(ctx, cfg, "eval")
	if err != nil {
		return nil, fmt.Errorf("build eval sub-agent: %w", err)
	}
	analysisAgent, err := buildSubAgent(ctx, cfg, "analysis")
	if err != nil {
		return nil, fmt.Errorf("build analysis sub-agent: %w", err)
	}

	strParam := func(desc string) *schema.ParameterInfo {
		return &schema.ParameterInfo{Type: schema.String, Desc: desc, Required: true}
	}
	optStrParam := func(desc string) *schema.ParameterInfo {
		return &schema.ParameterInfo{Type: schema.String, Desc: desc, Required: false}
	}

	// 将每个子 agent 包装为带显式输入 schema 的工具。
	codeTool := adk.NewAgentTool(ctx, codeAgent,
		adk.WithAgentInputSchema(schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task":              strParam("用户原始任务描述"),
			"requirements_path": strParam("需求文件相对路径，如 .spec/REQ-00001.md"),
			"failure_items":     optStrParam("上轮 eval 的不通过项，首轮传空字符串"),
		})),
	)
	evalTool := adk.NewAgentTool(ctx, evalAgent,
		adk.WithAgentInputSchema(schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task":              strParam("用户原始任务描述"),
			"requirements_path": strParam("需求文件相对路径，如 .spec/REQ-00001.md"),
		})),
	)
	analysisTool := adk.NewAgentTool(ctx, analysisAgent,
		adk.WithAgentInputSchema(schema.NewParamsOneOfByParams(map[string]*schema.ParameterInfo{
			"task":              strParam("用户原始任务描述"),
			"requirements_path": strParam("需求文件相对路径，如 .spec/REQ-00001.md"),
			"score":             optStrParam("eval 评分，如 85"),
		})),
	)

	buildAgentDef, err := agents.Build("build", cfg)
	if err != nil {
		return nil, err
	}
	scCfg := cfg.Agent["build"]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Model.APIKey,
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("create build chat model: %w", err)
	}

	pb := buildAgentDef.PromptBuilder()
	compressor := newContextCompressor(cfg.Model.MaxContextTokens, cfg.Model.SmartCompressThreshold, model, nil) // build agent 的压缩通知通过 notifyRef 路由
	planner, err := planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ToolCallingChatModel: model,
		GenInputFn: func(_ context.Context, userInput []adk.Message) ([]adk.Message, error) {
			return append([]adk.Message{schema.SystemMessage(pb.Planner())}, userInput...), nil
		},
	})
	if err != nil {
		return nil, err
	}
	executor, err := planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model: model,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{
				Tools:               []tool.BaseTool{codeTool, evalTool, analysisTool},
				ExecuteSequentially: true,
			},
			EmitInternalEvents: true,
		},
		GenInputFn: func(genCtx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			taskText, planJSON, executedJSON := extractExecutionContext(in)
			systemPrompt := pb.Executor()
			taskPrompt := fmt.Sprintf("用户任务:\n%s\n\n当前计划(JSON):\n%s", taskText, planJSON)
			executedJSON, _ = compressor.CompressExecutedSteps(genCtx, systemPrompt, taskPrompt, executedJSON)
			userPrompt := fmt.Sprintf("%s\n\n已执行步骤(JSON):\n%s", taskPrompt, executedJSON)
			return []adk.Message{schema.SystemMessage(systemPrompt), schema.UserMessage(userPrompt)}, nil
		},
		MaxIterations: scCfg.ExecutorMaxIterations,
	})
	if err != nil {
		return nil, err
	}
	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: model,
		GenInputFn: func(genCtx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			_, planJSON, executedJSON := extractExecutionContext(in)
			systemPrompt := pb.Replanner()
			taskPrompt := fmt.Sprintf("当前计划(JSON):\n%s", planJSON)
			executedJSON, _ = compressor.CompressExecutedSteps(genCtx, systemPrompt, taskPrompt, executedJSON)
			userPrompt := fmt.Sprintf("%s\n\n已执行步骤(JSON):\n%s\n\n基于以上执行情况判断是否重规划。", taskPrompt, executedJSON)
			return []adk.Message{schema.SystemMessage(systemPrompt), schema.UserMessage(userPrompt)}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	agentInst, err := planexecute.New(ctx, &planexecute.Config{
		Planner:       planner,
		Executor:      executor,
		Replanner:     replanner,
		MaxIterations: scCfg.PlanExecuteMaxIterations,
	})
	if err != nil {
		return nil, err
	}

	var notifyHolder ProgressFunc
	compressor.notify = func(msg string) {
		if notifyHolder != nil {
			notifyHolder(EventLog{AgentName: "system", Output: msg})
		}
	}
	svc := &Service{
		cfg:    cfg,
		agent:  buildAgentDef,
		runner: adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true}),
	}
	svc.notifyRef = &notifyHolder
	return svc, nil
}

// buildSubAgent 构建一个 planexecute adk.Agent，用于作为子 agent 工具。
func buildSubAgent(ctx context.Context, cfg *config.Config, agentName string) (adk.Agent, error) {
	agentDef, err := agents.Build(agentName, cfg)
	if err != nil {
		return nil, err
	}
	scCfg := cfg.Agent[agentName]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  cfg.Model.APIKey,
		BaseURL: cfg.Model.BaseURL,
		Model:   cfg.Model.Model,
	})
	if err != nil {
		return nil, fmt.Errorf("create %s chat model: %w", agentName, err)
	}
	tools, err := toolkit.BuildTools(cfg.Base.WorkspaceRoot, cfg.Base.CmdTimeoutSec, agentDef.ToolPolicy())
	if err != nil {
		return nil, err
	}
	inner, err := buildPlanExecuteAgent(ctx, model, agentDef.PromptBuilder(), tools, scCfg, cfg.Model.MaxContextTokens, cfg.Model.SmartCompressThreshold, nil)
	if err != nil {
		return nil, err
	}
	// 包装唯一名称，使 adk.NewAgentTool 为每个子 agent 生成不同的工具名。
	return &namedAgent{Agent: inner, name: agentName, desc: agentName + " sub-agent"}, nil
}

// namedAgent 包装 adk.Agent 并覆盖 Name/Description，
// 确保 adk.NewAgentTool 为每个子 agent 生成唯一的工具名。
type namedAgent struct {
	adk.Agent
	name string
	desc string
}

func (n *namedAgent) Name(_ context.Context) string        { return n.name }
func (n *namedAgent) Description(_ context.Context) string { return n.desc }

// buildPlanExecuteAgent is the shared factory for planner+executor+replanner agents.
func buildPlanExecuteAgent(
	ctx context.Context,
	chatModel einomodel.ToolCallingChatModel,
	pb agents.PromptBuilder,
	tools []tool.BaseTool,
	scCfg config.AgentConfig,
	maxContextTokens int,
	smartThreshold int,
	notify CompressNotify,
) (adk.Agent, error) {
	compressor := newContextCompressor(maxContextTokens, smartThreshold, chatModel, notify)

	planner, err := planexecute.NewPlanner(ctx, &planexecute.PlannerConfig{
		ToolCallingChatModel: chatModel,
		GenInputFn: func(_ context.Context, userInput []adk.Message) ([]adk.Message, error) {
			return append([]adk.Message{schema.SystemMessage(pb.Planner())}, userInput...), nil
		},
	})
	if err != nil {
		return nil, err
	}
	executor, err := planexecute.NewExecutor(ctx, &planexecute.ExecutorConfig{
		Model: chatModel,
		ToolsConfig: adk.ToolsConfig{
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools, ExecuteSequentially: true},
		},
		GenInputFn: func(genCtx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			taskText, planJSON, executedJSON := extractExecutionContext(in)
			systemPrompt := pb.Executor()
			taskPrompt := fmt.Sprintf("用户任务:\n%s\n\n当前计划(JSON):\n%s", taskText, planJSON)
			executedJSON, _ = compressor.CompressExecutedSteps(genCtx, systemPrompt, taskPrompt, executedJSON)
			userPrompt := fmt.Sprintf("%s\n\n已执行步骤(JSON):\n%s", taskPrompt, executedJSON)
			return []adk.Message{schema.SystemMessage(systemPrompt), schema.UserMessage(userPrompt)}, nil
		},
		MaxIterations: scCfg.ExecutorMaxIterations,
	})
	if err != nil {
		return nil, err
	}
	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: chatModel,
		GenInputFn: func(genCtx context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			_, planJSON, executedJSON := extractExecutionContext(in)
			systemPrompt := pb.Replanner()
			taskPrompt := fmt.Sprintf("当前计划(JSON):\n%s", planJSON)
			executedJSON, _ = compressor.CompressExecutedSteps(genCtx, systemPrompt, taskPrompt, executedJSON)
			userPrompt := fmt.Sprintf("%s\n\n已执行步骤(JSON):\n%s\n\n基于以上执行情况判断是否重规划。", taskPrompt, executedJSON)
			return []adk.Message{schema.SystemMessage(systemPrompt), schema.UserMessage(userPrompt)}, nil
		},
	})
	if err != nil {
		return nil, err
	}
	return planexecute.New(ctx, &planexecute.Config{
		Planner:       planner,
		Executor:      executor,
		Replanner:     replanner,
		MaxIterations: scCfg.PlanExecuteMaxIterations,
	})
}

func extractExecutionContext(in *planexecute.ExecutionContext) (taskText, planJSON, executedJSON string) {
	executedJSON = "[]"
	if in == nil {
		return
	}
	parts := make([]string, 0, len(in.UserInput))
	for _, m := range in.UserInput {
		if m != nil && strings.TrimSpace(m.Content) != "" {
			parts = append(parts, m.Content)
		}
	}
	taskText = strings.Join(parts, "\n")
	if in.Plan != nil {
		if b, err := in.Plan.MarshalJSON(); err == nil {
			planJSON = string(b)
		}
	}
	if b, err := json.Marshal(in.ExecutedSteps); err == nil {
		executedJSON = string(b)
	}
	return
}
