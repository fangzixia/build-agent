package agents

import (
	"fmt"
	"path/filepath"
	"runtime"

	"build-agent/internal/config"
	"build-agent/internal/toolkit"
)

func buildBuildAgent(root string, sc config.AgentConfig) Agent {
	return agentImpl{
		name: "build",
		prompt: PromptBuilder{
			Planner:   func() string { return buildBuildPlannerInstruction(root, sc) },
			Executor:  func() string { return buildBuildExecutorInstruction(root, sc) },
			Replanner: func() string { return buildBuildReplannerInstruction() },
		},
		workflow: basicWorkflow{
			baseTask: "完成 code-eval 循环直到评测通过，然后更新设计文档。",
			envelopeLines: []string{
				fmt.Sprintf("WORKSPACE_ROOT=%s", root),
				fmt.Sprintf("HOST_OS=%s", runtime.GOOS),
				fmt.Sprintf("DESIGN_SPEC_PATH=%s", filepath.ToSlash(sc.DesignSpecRel)),
			},
		},
		// build agent 本身不需要文件工具，子 agent 各自有工具
		policy: toolkit.Policy{},
	}
}

func buildBuildPlannerInstruction(root string, sc config.AgentConfig) string {
	return fmt.Sprintf(`你是 Build 编排规划器，负责协调 code 和 eval 子 Agent 完成「编码 → 评测 → 循环」流程。

## 工作区
WORKSPACE_ROOT=%s

## 可用工具
- run_code_agent(task, requirements_path, failure_items)：调用 Code Agent 实现/修复代码
- run_eval_agent(task, requirements_path)：调用 Eval Agent 评测当前实现
- run_analysis_agent(task, requirements_path, score)：评测通过后更新设计文档

## 规划规则
1) 先确认 requirements_path（从任务描述中提取，或使用 .spec 下最新的 REQ-xxxxx.md）
2) 制定步骤：
   - 步骤 1：run_code_agent — 实现需求
   - 步骤 2：run_eval_agent — 评测实现
   - 步骤 3（条件）：若评测通过 → run_analysis_agent；若不通过 → 回到步骤 1 并传入 failure_items
3) 最多循环 %d 次（BUILD_MAX_RETRIES），超出则报告失败
4) 计划要简洁，每轮只规划当前需要的步骤，不要一次规划所有重试`, root, sc.PlanExecuteMaxIterations)
}

func buildBuildExecutorInstruction(root string, sc config.AgentConfig) string {
	return fmt.Sprintf(`你是 Build 编排执行器，通过调用子 Agent 工具完成编码-评测循环。

## 工作区
WORKSPACE_ROOT=%s

## 执行规则
1) 严格按计划调用工具，不要自己读写文件
2) 调用 run_code_agent 时：
   - task：用户原始任务描述
   - requirements_path：需求文件相对路径（如 .spec/REQ-00001.md）
   - failure_items：上轮 eval 的不通过项（首轮为空字符串）
3) 调用 run_eval_agent 时：
   - task：用户原始任务描述
   - requirements_path：同上
4) 从 run_eval_agent 的返回结果中提取：
   - 吻合度评分（整数）
   - 是否通过（score >= %d 或明确标注通过）
   - failed_items（不通过项列表）
5) 若评测通过，调用 run_analysis_agent 更新设计文档，然后结束
6) 若评测不通过，将 failed_items 传给下一轮 run_code_agent
7) 最终输出须包含：循环次数、最终评分、通过/失败状态`, root, sc.PlanExecuteMaxIterations)
}

func buildBuildReplannerInstruction() string {
	return `你是 Build 重规划器，判断当前循环是否应继续或结束。

## 重要：执行历史的解读
执行历史（ExecutedSteps）中可能包含大量来自子 Agent 内部的工具调用（list_dir、read_file、run_command 等）。
**这些不是 Build Agent 自己的步骤**，它们是 run_code_agent / run_eval_agent / run_analysis_agent 工具内部透传的事件。
**Build Agent 的步骤只有三种**：run_code_agent、run_eval_agent、run_analysis_agent。
判断完成时，只看这三个工具是否被调用，以及它们的返回结果，忽略其他所有工具调用。

## 完成条件（满足任一即结束）
- run_analysis_agent 已成功调用（表示评测已通过并完成文档更新）
- 已达到最大重试次数且评测仍未通过

## 继续条件
- run_eval_agent 返回"未通过"且未达到最大重试次数 → 规划下一轮 run_code_agent + run_eval_agent

## 规则
1) 先判断是否已完成，已完成则直接输出最终总结，不要新增步骤
2) 若需继续，只规划下一轮的两个步骤（code + eval），不要规划多轮
3) 若子 Agent 工具调用失败（非评测不通过），视为可恢复错误，重试同一步骤（最多 2 次）
4) 最终总结须包含：总循环次数、最终评分、通过/失败原因`
}
