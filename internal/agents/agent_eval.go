package agents

import (
	"fmt"
	"path/filepath"
	"runtime"

	"build-agent/internal/config"
	"build-agent/internal/toolkit"
)

func buildEvalAgent(root string, sc config.AgentConfig, timeoutSec int) Agent {
	return agentImpl{
		name: "eval",
		prompt: PromptBuilder{
			Planner: func() string {
				return buildEvalPlannerInstruction(root, sc.DesignSpecRel, sc.RequirementsSpecRel, timeoutSec)
			},
			Executor: func() string {
				return buildEvalExecutorInstruction(root, sc.DesignSpecRel, sc.RequirementsSpecRel, timeoutSec)
			},
			Replanner: func() string {
				return buildEvalReplannerInstruction(timeoutSec)
			},
		},
		workflow: basicWorkflow{
			baseTask: "对照 requirements 评测 Web 项目实现并输出评测结论：读取 DESIGN_SPEC_PATH 与 REQUIREMENTS_SPEC_PATH，以用户视角验证页面交互、数据流转、前后端契约对齐、错误处理，结合代码与命令证据打分，在最终输出中给出吻合度评分与结构化结论；未通过时必须列出具体不通过项（failed_items[]、fix_priority[]），供 code 阶段回流修复。",
			envelopeLines: []string{
				fmt.Sprintf("WORKSPACE_ROOT=%s", root),
				fmt.Sprintf("HOST_OS=%s", runtime.GOOS),
				fmt.Sprintf("CMD_TIMEOUT_SEC=%d", timeoutSec),
				fmt.Sprintf("DESIGN_SPEC_PATH=%s", filepath.ToSlash(sc.DesignSpecRel)),
				fmt.Sprintf("DESIGN_SPEC_ABS=%s", sc.DesignSpecAbs),
				fmt.Sprintf("REQUIREMENTS_SPEC_PATH=%s", filepath.ToSlash(sc.RequirementsSpecRel)),
				fmt.Sprintf("REQUIREMENTS_SPEC_ABS=%s", sc.RequirementsSpecAbs),
				"TEMP_DIR=.eval-agent-tmp",
				"EVALUATION_OUTPUT_DIR=.spec",
				"EVALUATION_OUTPUT_PATTERN=EVAL-REQ-xxxxx-xx.md",
			},
		},
		policy: toolkit.Policy{
			TempDirName:           ".eval-agent-tmp",
			AllowRunCommand:       true,
			MissingPathAsExistsNo: true,
			WriteAllowPrefixes:    []string{".eval-agent-tmp", ".spec/EVAL-"},
			EnablePythonRewrite:   true,
			EnablePortTools:       true,
		},
		postProcessor: evalPostProcessor{
			workspaceRoot: root,
			passThreshold: 100,
		},
	}
}

func commandResilienceHint(cmdTimeoutSec int) string {
	if cmdTimeoutSec <= 0 {
		cmdTimeoutSec = 90
	}
	return fmt.Sprintf(`命令执行与重试策略（宿主保证单条 run_command 最长约 %d 秒）：
- run_command 同步等待子进程结束；服务类进程必须用 start_background 工具，不得用 run_command 启动。
- exit_code=124 表示超时，须立即换策略（静态分析或缩小验证范围）。
- exit_code 非 0 时先读 stdout/stderr 判断原因，可换命令或改为静态审查。
- HTTP 探测失败（exit_code=7 或非 2xx）表示服务未就绪，不等于工具损坏；确认服务状态后再决定是否重试。
- 同类 start_background 失败 ≥2 次或权限拒绝时，停止再试，改为静态对照，相关 AC 标注 BLOCKED。`, cmdTimeoutSec)
}

func serviceLifecyclePolicy() string {
	return `## 服务生命周期管理

**专用工具**（无需 run_command 执行 netstat/taskkill 等）：
- ` + "`check_port(port)`" + `：检查端口占用，返回 PIDs
- ` + "`kill_port(port)`" + `：终止占用指定端口的进程
- ` + "`start_background(command, cwd, log_path)`" + `：后台启动服务，立即返回，输出写入 log_path

**流程**：
1. 启动前调用 ` + "`check_port`" + `；若占用则先 ` + "`kill_port`" + ` 再启动。
2. 用 ` + "`start_background`" + ` 启动服务（禁止用 run_command 启动常驻进程）。
3. 评测结束后对本次启动的每个端口调用 ` + "`kill_port`" + ` 清理。`
}

func nonBlockingCommandPolicy() string {
	return `## 阻塞命令禁令
- **禁止**用 run_command 执行会常驻的服务进程（java -jar、mvn spring-boot:run、npm start、go run 等）；宿主会直接拒绝并提示改用 start_background。
- **Windows**：禁止用 run_command 执行 Start-Process、start /B 等后台启动命令（PowerShell 执行器会同步等待）；必须用 start_background 工具。
- 若仅需验证环境/语法，只用会自行退出的命令（java -version、node -v、mvn --version 等）。`
}

func evalInferenceAndExplorationBlock() string {
	return `## 推断式项目识别与探索预算
1) 先 read_file 设计说明与需求清单；REQ 编号以 .spec 下实际文件为准，不得臆造。
2) list_dir 工作区根，根据实际构建/依赖文件推断技术栈；逐级 list_dir 合计不超过约 5 步。
3) 禁止预设技术栈或固定目录结构；若 design/REQ 已给出路径，优先 read_file。
4) 若需在线验证：用 start_background 启动服务，再用短命令探测；失败则改为静态审查。`
}

func hostRuntimeHint() string {
	goos := runtime.GOOS
	switch goos {
	case "windows":
		return fmt.Sprintf(`宿主执行环境：Windows（GOOS=%s）。
run_command 通过 PowerShell 执行；禁止在命令里嵌套 cmd /c 或 Start-Process。
后台启动服务必须用 start_background 工具，不得用 run_command。
勿使用 Unix 习惯命令（find、grep、head 等）。`, goos)
	case "darwin":
		return fmt.Sprintf(`宿主执行环境：macOS（GOOS=%s）。run_command 通过 sh -lc 执行。后台启动用 start_background 工具。`, goos)
	default:
		return fmt.Sprintf(`宿主执行环境：类 Unix（GOOS=%s）。run_command 通过 sh -lc 执行。后台启动用 start_background 工具。`, goos)
	}
}

func buildEvalPlannerInstruction(workspaceRoot, designSpecRel, requirementsSpecRel string, cmdTimeoutSec int) string {
	d := filepath.ToSlash(designSpecRel)
	r := filepath.ToSlash(requirementsSpecRel)
	return fmt.Sprintf(`你是项目需求吻合度评测规划器。

## 目标
将评测任务拆分为可执行步骤，最终在执行器输出中给出完整评测结论（含吻合度评分）。
临时文件（服务日志、中间笔记等）写入 .eval-agent-tmp/，任务结束后宿主自动清理。

%s

## 规则
1) 步骤：读取 design（%s）与 requirements（%s）→ 浅层探索代码 → 按需启动服务验证 → 对照 AC 打分举证 → 在最终输出中给出结论。
2) 工作区约束：%s。
3) 不需要写草稿文件；评测结论直接在最终输出中给出。
4) 涉及启动服务：用 start_background + 短探测；失败达止损次数时改为静态分析。
5) 评测主视角：系统实际用户视角（页面流程、交互反馈、前后端接口契约）。
6) 结果须含：failed_items[]、evidence_refs[]、fix_priority[]，AC-ID 逐条判定。

%s

%s

%s

%s`, evalInferenceAndExplorationBlock(), d, r, workspaceRoot, commandResilienceHint(cmdTimeoutSec), nonBlockingCommandPolicy(), serviceLifecyclePolicy(), hostRuntimeHint())
}

func buildEvalExecutorInstruction(workspaceRoot, designSpecRel, requirementsSpecRel string, cmdTimeoutSec int) string {
	d := filepath.ToSlash(designSpecRel)
	r := filepath.ToSlash(requirementsSpecRel)
	return fmt.Sprintf(`你是项目需求吻合度评测 Agent。

%s

## 规则
1) 先 list_dir 根目录与 read_file design/REQ，再按需浅层探索，再下结论。
2) 工作区约束：%s。
3) 必须实际调用工具；不允许只输出计划性套话。
4) 主输入：设计说明（%s）；需求清单（%s）。
5) **评测结论直接在最终输出中给出**，不需要写草稿文件。临时笔记用 write_temp_file 写入 .eval-agent-tmp/。
6) 服务启动用 start_background 工具；禁止用 run_command 启动常驻进程。
7) 最终输出必须包含：
   - **吻合度评分**：整数/100
   - 逐条 AC 判定：AC-ID | REQ-ID | 结果(PASS/FAIL/BLOCKED) | 证据摘要
   - failed_items[]：fail_id、req_id、ac_id、reason、evidence_ref、suggested_fix
   - evidence_refs[]：命令输出/代码位置/文档片段
   - fix_priority[]：FAIL-ID 优先级列表
   - status：DONE / BLOCKED / NEEDS_CLARIFICATION
8) 评测口径：用户视角（页面交互、可感知一致性、前后端接口统一性）；无法在线联调时用静态证据替代并标注风险。
9) 正式报告由宿主自动写入 .spec/EVAL-REQ-xxxxx-xx.md，无需手动写入。

%s

%s

%s

%s`, evalInferenceAndExplorationBlock(), workspaceRoot, d, r, commandResilienceHint(cmdTimeoutSec), nonBlockingCommandPolicy(), serviceLifecyclePolicy(), hostRuntimeHint())
}

func buildEvalReplannerInstruction(cmdTimeoutSec int) string {
	return fmt.Sprintf(`你是重规划 Agent。

## 规则
1) **完成判断**：执行器输出中已包含「吻合度评分」整数分数，且有 AC-ID 级判定与 status 字段，则视为已完成，直接给出最终响应，不要继续新增步骤。
2) 只有在结论不完整或执行失败时才重规划。exit_code=124 或非 0 视为可恢复，缩小验证范围或改为静态对照。
3) start_background 同类失败 ≥2 次或权限拒绝时，停止再试，改为静态对照，相关 AC 标注 BLOCKED。
4) 重规划时最小改动；不得安排前台阻塞命令；不得在止损后继续追加同类启动。
5) 若输出缺少结构化字段（failed_items[] / evidence_refs[] / fix_priority[] / status）或缺少 AC-ID 级判定，须补齐后结束。

%s

%s

%s

%s`, commandResilienceHint(cmdTimeoutSec), nonBlockingCommandPolicy(), serviceLifecyclePolicy(), hostRuntimeHint())
}

// hasEvalScore returns true if the content contains an integer evaluation score.
func hasEvalScore(content string) bool {
	return len(content) > 0 && (containsStr(content, "吻合度评分") || containsStr(content, "/100"))
}

func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && findStr(s, sub))
}

func findStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
