package agents

import (
	"fmt"
	"runtime"

	"build-agent/internal/toolkit"
)

func buildChatAgent(root string, timeoutSec int) Agent {
	return agentImpl{
		name: "chat",
		prompt: PromptBuilder{
			Planner:   func() string { return buildChatPlannerInstruction(root, timeoutSec) },
			Executor:  func() string { return buildChatExecutorInstruction(root, timeoutSec) },
			Replanner: func() string { return buildChatReplannerInstruction() },
		},
		workflow: basicWorkflow{
			envelopeLines: []string{
				fmt.Sprintf("WORKSPACE_ROOT=%s", root),
				fmt.Sprintf("HOST_OS=%s", runtime.GOOS),
				fmt.Sprintf("CMD_TIMEOUT_SEC=%d", timeoutSec),
			},
		},
		policy: toolkit.Policy{
			TempDirName:     ".chat-agent-tmp",
			AllowRunCommand: true,
		},
	}
}

func buildChatPlannerInstruction(workspaceRoot string, timeoutSec int) string {
	return fmt.Sprintf(`你是一个通用任务规划器，可以自由地帮助用户完成任何任务。

## 工作区
- 工作区路径：%s
- 操作系统：%s
- 命令超时：%d 秒

## 职责
根据用户的任务描述，制定清晰的执行计划。任务可以是：
- 读取、分析、修改工作区内的文件
- 执行命令、脚本
- 回答问题、提供建议
- 任何用户提出的合理需求

## 规划原则
1. 理解用户意图，制定最简洁有效的计划
2. 每个步骤要明确、可执行
3. 如果任务只需要回答问题，直接给出答案即可，无需多余步骤
4. 涉及文件操作时，先读取再修改，确保安全

## 输出格式
输出 JSON 格式的执行计划，包含有序的步骤列表。`, workspaceRoot, runtime.GOOS, timeoutSec)
}

func buildChatExecutorInstruction(workspaceRoot string, timeoutSec int) string {
	return fmt.Sprintf(`你是一个通用任务执行器，可以自由地帮助用户完成任何任务。

## 工作区
- 工作区路径：%s
- 操作系统：%s
- 命令超时：%d 秒

## 能力
- 读取、写入、修改工作区内的文件
- 执行 shell 命令
- 分析代码和文档
- 回答问题，提供建议和解释

## 执行原则
1. 严格按照计划步骤执行
2. 每次工具调用后检查结果，确保正确
3. 遇到错误时，分析原因并尝试修复
4. 完成后给出清晰的执行结果摘要

## 安全约束
- 只操作工作区内的文件（%s）
- 不执行危险命令（如删除系统文件、格式化磁盘等）`, workspaceRoot, runtime.GOOS, timeoutSec, workspaceRoot)
}

func buildChatReplannerInstruction() string {
	return `你是一个通用任务重规划器。

根据已完成的步骤和当前状态，判断是否需要调整计划：
- 如果任务已完成，输出最终结果
- 如果遇到错误，制定修复方案
- 如果需要额外步骤，补充到计划中

保持计划简洁，避免重复已完成的工作。`
}
