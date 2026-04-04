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
	if agentName == "build" {
		return newBuildAgentService(ctx, cfg)
	}
	return newPlanExecuteService(ctx, cfg, agentName)
}

func newPlanExecuteService(ctx context.Context, cfg *config.Config, agentName string) (*Service, error) {
	scn, err := agents.Build(agentName, cfg)
	if err != nil {
		return nil, err
	}
	scCfg := cfg.Agent[agentName]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  scCfg.OpenAIAPIKey,
		BaseURL: scCfg.OpenAIBaseURL,
		Model:   scCfg.OpenAIModel,
	})
	if err != nil {
		return nil, fmt.Errorf("create chat model: %w", err)
	}
	tools, err := toolkit.BuildTools(cfg.Base.WorkspaceRoot, cfg.Base.CmdTimeoutSec, scn.ToolPolicy())
	if err != nil {
		return nil, err
	}
	pb := scn.PromptBuilder()
	agentInst, err := buildPlanExecuteAgent(ctx, model, pb, tools, scCfg)
	if err != nil {
		return nil, err
	}
	return &Service{
		cfg:    cfg,
		agent:  scn,
		runner: adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true}),
	}, nil
}

// newBuildAgentService builds a planexecute Service for the "build" agent,
// where code/eval/analysis sub-agents are exposed as tools to the build executor.
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

	// Wrap each sub-agent as a tool with an explicit input schema.
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
		APIKey:  scCfg.OpenAIAPIKey,
		BaseURL: scCfg.OpenAIBaseURL,
		Model:   scCfg.OpenAIModel,
	})
	if err != nil {
		return nil, fmt.Errorf("create build chat model: %w", err)
	}

	pb := buildAgentDef.PromptBuilder()
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
		GenInputFn: func(_ context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			taskText, planJSON, executedJSON := extractExecutionContext(in)
			userPrompt := fmt.Sprintf("用户任务:\n%s\n\n当前计划(JSON):\n%s\n\n已执行步骤(JSON):\n%s", taskText, planJSON, executedJSON)
			// Only pass system + user messages; sub-agent internal messages (tool-call-only
			// assistant messages with empty content) must not be forwarded to the LLM.
			return []adk.Message{schema.SystemMessage(pb.Executor()), schema.UserMessage(userPrompt)}, nil
		},
		MaxIterations: scCfg.ExecutorMaxIterations,
	})
	if err != nil {
		return nil, err
	}
	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: model,
		GenInputFn: func(_ context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			_, planJSON, executedJSON := extractExecutionContext(in)
			userPrompt := fmt.Sprintf("当前计划(JSON):\n%s\n\n已执行步骤(JSON):\n%s\n\n基于以上执行情况判断是否重规划。", planJSON, executedJSON)
			return []adk.Message{schema.SystemMessage(pb.Replanner()), schema.UserMessage(userPrompt)}, nil
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
	return &Service{
		cfg:    cfg,
		agent:  buildAgentDef,
		runner: adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true}),
	}, nil
}

// buildSubAgent constructs a planexecute adk.Agent for use as a sub-agent tool.
func buildSubAgent(ctx context.Context, cfg *config.Config, agentName string) (adk.Agent, error) {
	agentDef, err := agents.Build(agentName, cfg)
	if err != nil {
		return nil, err
	}
	scCfg := cfg.Agent[agentName]
	model, err := openai.NewChatModel(ctx, &openai.ChatModelConfig{
		APIKey:  scCfg.OpenAIAPIKey,
		BaseURL: scCfg.OpenAIBaseURL,
		Model:   scCfg.OpenAIModel,
	})
	if err != nil {
		return nil, fmt.Errorf("create %s chat model: %w", agentName, err)
	}
	tools, err := toolkit.BuildTools(cfg.Base.WorkspaceRoot, cfg.Base.CmdTimeoutSec, agentDef.ToolPolicy())
	if err != nil {
		return nil, err
	}
	inner, err := buildPlanExecuteAgent(ctx, model, agentDef.PromptBuilder(), tools, scCfg)
	if err != nil {
		return nil, err
	}
	// Wrap with a unique name so adk.NewAgentTool produces distinct tool names.
	return &namedAgent{Agent: inner, name: agentName, desc: agentName + " sub-agent"}, nil
}

// namedAgent wraps an adk.Agent and overrides Name/Description so that
// adk.NewAgentTool produces a unique tool name for each sub-agent.
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
) (adk.Agent, error) {
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
		GenInputFn: func(_ context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			taskText, planJSON, executedJSON := extractExecutionContext(in)
			userPrompt := fmt.Sprintf("用户任务:\n%s\n\n当前计划(JSON):\n%s\n\n已执行步骤(JSON):\n%s", taskText, planJSON, executedJSON)
			return []adk.Message{schema.SystemMessage(pb.Executor()), schema.UserMessage(userPrompt)}, nil
		},
		MaxIterations: scCfg.ExecutorMaxIterations,
	})
	if err != nil {
		return nil, err
	}
	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: chatModel,
		GenInputFn: func(_ context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			_, planJSON, executedJSON := extractExecutionContext(in)
			userPrompt := fmt.Sprintf("当前计划(JSON):\n%s\n\n已执行步骤(JSON):\n%s\n\n基于以上执行情况判断是否重规划。", planJSON, executedJSON)
			return []adk.Message{schema.SystemMessage(pb.Replanner()), schema.UserMessage(userPrompt)}, nil
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
