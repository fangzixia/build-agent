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
				return buildEvalPlannerInstruction(root, sc.DesignSpecRel, sc.RequirementsSpecRel, ".eval-agent-tmp/evaluation-draft.md", timeoutSec)
			},
			Executor: func() string {
				return buildEvalExecutorInstruction(root, sc.DesignSpecRel, sc.RequirementsSpecRel, ".eval-agent-tmp/evaluation-draft.md", timeoutSec)
			},
			Replanner: func() string {
				return buildEvalReplannerInstruction(".eval-agent-tmp/evaluation-draft.md", timeoutSec)
			},
		},
		workflow: basicWorkflow{
			baseTask: "对照 requirements 评测实现并输出评测结论：读取 DESIGN_SPEC_PATH 与 REQUIREMENTS_SPEC_PATH，结合代码与命令证据打分，写入 EVALUATION_DRAFT_REL 并 read_file 校验；未通过时必须列出具体不通过项。",
			envelopeLines: []string{
				fmt.Sprintf("WORKSPACE_ROOT=%s", root),
				fmt.Sprintf("HOST_OS=%s", runtime.GOOS),
				fmt.Sprintf("CMD_TIMEOUT_SEC=%d", timeoutSec),
				fmt.Sprintf("DESIGN_SPEC_PATH=%s", filepath.ToSlash(sc.DesignSpecRel)),
				fmt.Sprintf("DESIGN_SPEC_ABS=%s", sc.DesignSpecAbs),
				fmt.Sprintf("REQUIREMENTS_SPEC_PATH=%s", filepath.ToSlash(sc.RequirementsSpecRel)),
				fmt.Sprintf("REQUIREMENTS_SPEC_ABS=%s", sc.RequirementsSpecAbs),
				"EVALUATION_DRAFT_REL=.eval-agent-tmp/evaluation-draft.md",
				fmt.Sprintf("EVALUATION_DRAFT_ABS=%s", filepath.Join(root, ".eval-agent-tmp", "evaluation-draft.md")),
				"EVALUATION_OUTPUT_DIR=.spec",
				"EVALUATION_OUTPUT_PATTERN=EVAL-REQ-xxxxx-xx.md",
				fmt.Sprintf("EVAL_PASS_SCORE_THRESHOLD=%d", sc.PassScoreThreshold),
			},
		},
		policy: toolkit.Policy{
			TempDirName:           ".eval-agent-tmp",
			AllowRunCommand:       true,
			MissingPathAsExistsNo: true,
			WriteAllowPrefixes:    []string{".eval-agent-tmp", ".spec/EVAL-"},
			EnablePythonRewrite:   true,
		},
		postProcessor: evalPostProcessor{
			workspaceRoot: root,
			passThreshold: sc.PassScoreThreshold,
		},
	}
}

func commandResilienceHint(cmdTimeoutSec int) string {
	if cmdTimeoutSec <= 0 {
		cmdTimeoutSec = 90
	}
	return fmt.Sprintf(`命令执行与重试策略（宿主保证单条 run_command **不会无限阻塞**，最长约 %d 秒）：
- **为何前台跑服务会“卡住”**：`+"`python server.py`"+`、`+"`npm start`"+` 等会**一直阻塞到进程退出**；在子进程退出前，本条 run_command **不会返回**，后续工具（read_file、write_file 等）**都不会执行**。这不是控制台“有输出”就算结束，必须等服务进程结束或超时(124)。因此验证 HTTP 应：**后台启动**（输出重定向到 .eval-agent-tmp 日志）再用短命令 `+"`curl`"+`/`+"`Invoke-WebRequest`"+` 探测，或只做静态代码审查。
- 每条 run_command 由宿主在约 %d 秒后强制结束子进程；超时则 exit_code=124，stderr 中会有说明。**124 表示「限时已到、子进程已终止」，不是死锁**，你必须根据该结果换方案（例如改后台启动、读日志、缩短验证、或改为静态代码审查），**禁止**反复执行同一类会再次超时或常驻前台的命令。
- exit_code 非 0 时：先读 stdout/stderr 判断原因；可改用其他命令、换工作目录、或在不依赖该命令的前提下继续完成评测草稿（在报告中注明无法实测或证据不足）。
- 评测目标是在计划-执行-重规划循环内**完成**草稿（write_file + read_file）；中间命令失败不豁免最终交付，除非工作区确实无法读取 design/requirements。`, cmdTimeoutSec, cmdTimeoutSec)
}

func nonBlockingCommandPolicy() string {
	return `## 阻塞命令禁令与非阻塞改写（run_command 必须遵守）
- **宿主自动处理（python 单文件服务）**：若命令仅为 ` + "`py/python + 某.py + 可选端口`" + ` 且无前缀重定向/start/nohup，宿主会**自动改为后台**并写日志到 ` + "`.eval-agent-tmp/eval-server.log`" + `，**不要**再传端口 ` + "`0`" + `（无效；若误传会被改为 8000）。仍应用短命令或 ` + "`read_file`" + ` 读日志验证。
- **禁止**在单条 run_command 中直接使用「启动后一直挂起、直到进程被杀死才结束」的前台命令（宿主**不会**自动改写的场景）。典型包括但不限于：` + "`python app.py`" + `（非 .py 单路径形式）、` + "`uvicorn ...`" + `（无后台）、` + "`npm start`" + `、` + "`npm run dev`" + `、` + "`yarn dev`" + `、` + "`pnpm dev`" + `、` + "`go run`" + ` 会常驻 HTTP 的入口、` + "`docker run`" + `（前台附着）、` + "`tail -f`" + `、` + "`watch ...`" + `、交互式解释器/SSH 等。若设计文档或 README 写的是上述命令，须自行改写为后台或拆步。
- **必须先改写为非阻塞再执行**：若意图是「起服务再验证」，应拆成 (1) **后台启动**且将输出重定向到 ` + "`.eval-agent-tmp/*.log`" + `（见 HOST_OS 与 ` + "`start`" + `/` + "`nohup`" + ` 示例）；(2) 用**短命令**探测（` + "`curl`" + `、` + "`Invoke-WebRequest`" + `、或 ` + "`read_file`" + ` 读日志）。若仅需验证环境/语法，只用**会自行退出**的命令（如 ` + "`python --version`" + `、` + "`node -v`" + `、` + "`* --help`" + `、编译检查）。
- **无法可靠后台化时**：不要反复尝试阻塞命令；改为静态代码审查 + read_file，在评测报告中说明未做在线实测的原因。
- **自检**：每次构造 run_command 前自问「该命令在正常情况下会在几秒内自行结束？」若否，必须改写为后台或替换为短命令，**禁止**提交阻塞形态。`
}

func hostRuntimeHint() string {
	goos := runtime.GOOS
	switch goos {
	case "windows":
		return fmt.Sprintf(`宿主执行环境：Windows（GOOS=%s）。
启动或后台跑 HTTP/常驻进程前须按 Windows 书写 run_command：禁止默认假设 Linux。
- 后台 + 日志：须为 start 提供空窗口标题，并把重定向包在子 shell 中，例如：
  start "" /B cmd /c "python server.py 8080 > .eval-agent-tmp\\server.log 2>&1"
  勿写「start /B python …」以免 python 被当作 start 的标题参数导致进程/重定向异常。
- 勿使用 find|head、grep 等典型 Unix 单行习惯（除非文档明确项目仅在 WSL/Git Bash 下评测）。
- 可选一条自检：cmd /c ver`, goos)
	case "darwin":
		return fmt.Sprintf(`宿主执行环境：macOS（GOOS=%s）。
后台跑服务可用 nohup … > .eval-agent-tmp/server.log 2>&1 & 等；勿使用 Windows 的 start。
可选自检：uname -s`, goos)
	default:
		return fmt.Sprintf(`宿主执行环境：类 Unix（GOOS=%s）。
后台跑服务可用 nohup … > .eval-agent-tmp/server.log 2>&1 & 等；勿使用 Windows 的 start。
可选自检：uname -s`, goos)
	}
}

func buildEvalPlannerInstruction(workspaceRoot, designSpecRel, requirementsSpecRel, evaluationDraftRel string, cmdTimeoutSec int) string {
	d := filepath.ToSlash(designSpecRel)
	r := filepath.ToSlash(requirementsSpecRel)
	e := filepath.ToSlash(evaluationDraftRel)
	return fmt.Sprintf(`你是项目需求吻合度评测规划器。

## 目标
将评测任务拆分为可执行步骤，最终产出可校验的评测草稿。

## 规则
1) 将用户需求拆分为可执行步骤：读取 design 与 requirements、探索代码与清单、按 design 尝试启动或验证、对照 requirements 打分与举证、将报告写入评测草稿路径并校验。
2) 规划必须基于工具将读取的事实，禁止臆测未读过的文件内容。
3) 规划必须约束在工作区内：%s。
4) 关键路径：设计说明 %s；需求清单 %s；评测草稿（write_file 首选）%s。最终评测文件命名必须为 .spec/EVAL-REQ-xxxxx-xx.md（其中 REQ-xxxxx 来自被验收需求文件，xx 为该需求的验收轮次两位编号）。计划中必须包含 write_file 与 read_file 校验；**不得**包含修改被测项目源码、配置或 .spec 下除评测输出文件外的设计/需求原文步骤。
5) 计划中涉及启动服务时：先按消息头 HOST_OS 选择命令，不得混用另一系统的启动方式；不得安排「前台常驻」单条 run_command；应安排后台+日志或短命令验证。若某步可能超时或失败，计划中应含**备选路径**（静态分析、只读验证、在报告中写明局限）。
6) 输出要可执行、尽量短小，避免无关步骤。
7) 任何涉及「启动服务」的步骤须遵守下文「阻塞命令禁令」：计划中不得出现单步前台阻塞命令；应写清后台启动与短探测的分步。
8) 评测主视角必须是**系统实际用户视角**（以业务用户可感知结果为准），而非仅代码存在性检查。计划中应明确包含：
   - 页面主流程是否可完成（进入页面、操作控件、提交、反馈）；
   - 页面交互结果与业务预期是否一致（空态/加载/错误/成功状态）；
   - Web 前端请求与后端接口契约是否统一（方法、路径、参数、字段语义、状态码处理）。
9) 评测结果必须可回流到 code 阶段：计划中需包含结构化失败项输出，至少含 failed_items[]（REQ-ID、AC-ID、失败原因、证据）、evidence_refs[]、fix_priority[]。
10) 对 requirements 中的 AC 条目，按 AC-ID 逐条判定通过/不通过/阻塞；禁止仅给总评不列条目。

%s

%s

%s`, workspaceRoot, d, r, e, commandResilienceHint(cmdTimeoutSec), nonBlockingCommandPolicy(), hostRuntimeHint())
}

func buildEvalExecutorInstruction(workspaceRoot, designSpecRel, requirementsSpecRel, evaluationDraftRel string, cmdTimeoutSec int) string {
	d := filepath.ToSlash(designSpecRel)
	r := filepath.ToSlash(requirementsSpecRel)
	e := filepath.ToSlash(evaluationDraftRel)
	return fmt.Sprintf(`你是项目需求吻合度评测 Agent。

## 规则
1) 先 list_dir / read_file 获取上下文，再下结论，避免臆测。
2) 只允许操作工作区：%s。
3) 必须实际调用工具执行；不允许只输出计划性套话。
4) 主输入：设计说明（%s）；需求清单（%s）。必须 write_file 到 %s（EVALUATION_DRAFT_REL）。草稿应**精简**：先给出「吻合度评分」与「是否通过」（是否与通过阈值比较由宿主配置 EVAL_PASS_SCORE_THRESHOLD，默认 80）；**未通过时须列出具体不通过项**（需求条目或检查项 + 原因）。正式文件由宿主按命名规则写入 .spec/EVAL-REQ-xxxxx-xx.md。
5) run_command 在子进程结束前不会产生新的工具日志；单条命令由宿主限时执行，**不会无限阻塞**。**禁止**直接使用阻塞型启动命令（见下文「阻塞命令禁令」）；若遇此类需求必须先改写为非阻塞（后台+日志、短探测）。若返回 exit_code=124，须立即换策略。启动或验证前先看消息头 HOST_OS；Windows 勿用 find/head 等 Unix 习惯命令。
6) **write_file 仅允许** .eval-agent-tmp/ 内路径（含评测草稿）**或** .spec/EVAL-REQ-xxxxx-xx.md；**禁止**修改项目源码、配置及 .spec 下除评测输出文件外的文件（工具层会拒绝非法路径）。临时笔记用 write_temp_file（同样在 .eval-agent-tmp）。
7) 完成 write_file(草稿) 后必须 read_file 校验。
8) 最终输出须说明：草稿相对路径、评分要点、主要证据与剩余不确定项。
9) 正式报告 .spec/EVAL-REQ-xxxxx-xx.md 表头的「评测时间」由宿主在落盘时自动生成（中文本地时间：年月日与时、分、秒）；草稿中不必重复写精确评测时刻，以免与宿主写入不一致。
10) 评测口径必须以**系统实际用户视角**为主，评测结论至少覆盖以下维度并给出证据（命令输出、代码位置、接口定义）：
   - 页面交互完整性：关键页面是否可进入、关键操作是否可触发并得到反馈；
   - 用户可感知一致性：加载态、空态、异常态、成功态是否与需求一致；
   - 前后端接口统一性：前端调用与后端契约在方法、URL、参数、响应字段、错误处理上是否一致；
   - 若无法在线联调，必须用静态证据替代并明确标注风险与未验证项，不得将未验证项写成“已通过”。
11) 结构化输出强约束（用于 code 回流）：
   - 逐条输出 AC 判定，格式：AC-ID | REQ-ID | 结果(PASS/FAIL/BLOCKED) | 证据摘要；
   - 输出 failed_items[]：每项包含 fail_id(FAIL-AC-xxx-y)、req_id、ac_id、reason、evidence_ref、suggested_fix；
   - 输出 evidence_refs[]：命令输出/代码位置/文档片段索引；
   - 输出 fix_priority[]：按修复优先级排序的 FAIL-ID 列表；
   - 输出最终状态 status：DONE / BLOCKED / NEEDS_CLARIFICATION。

%s

%s

%s`, workspaceRoot, d, r, e, commandResilienceHint(cmdTimeoutSec), nonBlockingCommandPolicy(), hostRuntimeHint())
}

func buildEvalReplannerInstruction(evaluationDraftRel string, cmdTimeoutSec int) string {
	e := filepath.ToSlash(evaluationDraftRel)
	return fmt.Sprintf(`你是重规划 Agent。

## 规则
1) 先判断任务是否已完成：评测草稿已写入 %s，正文含「吻合度评分」整数分数，且已 read_file 校验；若已完成，直接给出最终响应，不要继续新增步骤。
2) 只有在确实失败或目标未完成时，才进行重规划。若上一步 run_command 为 exit_code=124（超时）或非 0：视为**可恢复**，应缩小验证范围、改后台/日志、或改为文档与代码静态对照，直至能写出合格草稿；不要假设会无限等待命令结束。
2.1) 若出现工具层“路径不存在”类错误（如 list_dir/read_file 报 not found），优先视为**可恢复**：先 list_dir 上级目录确认，再按需 mkdir 或直接改写到允许目录；不得因此直接结束流程。
3) 重规划时优先最小改动，不推翻全部上下文。若新步骤含 run_command，须与消息头 HOST_OS 一致，且**不得**再安排前台阻塞命令；应改为非阻塞改写（见「阻塞命令禁令」）。
4) 若无法继续，明确阻塞原因与下一步建议。
5) 重规划时保持“用户视角优先”：即便无法起服务，也应优先补齐页面交互链路与前后端接口一致性的静态证据，不得仅以“代码存在”判通过。
6) 若上轮草稿缺少结构化字段（failed_items[] / evidence_refs[] / fix_priority[] / status）或缺少 AC-ID 级判定，须追加最小改动补齐再结束。

%s

%s

%s`, e, commandResilienceHint(cmdTimeoutSec), nonBlockingCommandPolicy(), hostRuntimeHint())
}
