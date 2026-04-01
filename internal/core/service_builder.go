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
	"github.com/cloudwego/eino/compose"
	"github.com/cloudwego/eino/schema"
)

func NewService(ctx context.Context, cfg *config.Config, agentName string) (*Service, error) {
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
			ToolsNodeConfig: compose.ToolsNodeConfig{Tools: tools, ExecuteSequentially: true},
		},
		GenInputFn: func(_ context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			taskText := ""
			planJSON := ""
			executedJSON := "[]"
			if in != nil {
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
			}
			userPrompt := fmt.Sprintf("用户任务:\n%s\n\n当前计划(JSON):\n%s\n\n已执行步骤(JSON):\n%s", taskText, planJSON, executedJSON)
			return []adk.Message{
				schema.SystemMessage(pb.Executor()),
				schema.UserMessage(userPrompt),
			}, nil
		},
		MaxIterations: scCfg.ExecutorMaxIterations,
	})
	if err != nil {
		return nil, err
	}
	replanner, err := planexecute.NewReplanner(ctx, &planexecute.ReplannerConfig{
		ChatModel: model,
		GenInputFn: func(_ context.Context, in *planexecute.ExecutionContext) ([]adk.Message, error) {
			planJSON := ""
			executedJSON := "[]"
			if in != nil {
				if in.Plan != nil {
					if b, err := in.Plan.MarshalJSON(); err == nil {
						planJSON = string(b)
					}
				}
				if b, err := json.Marshal(in.ExecutedSteps); err == nil {
					executedJSON = string(b)
				}
			}
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
		agent:  scn,
		runner: adk.NewRunner(ctx, adk.RunnerConfig{Agent: agentInst, EnableStreaming: true}),
	}, nil
}
