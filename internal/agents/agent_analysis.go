package agents

import (
	"fmt"
	"path/filepath"

	"build-agent/internal/config"
	"build-agent/internal/toolkit"
)

func buildAnalysisAgent(root string, sc config.AgentConfig) Agent {
	return agentImpl{
		name: "analysis",
		prompt: PromptBuilder{
			Planner:   func() string { return buildAnalysisPlannerInstruction(root, sc.DesignSpecRel) },
			Executor:  func() string { return buildAnalysisExecutorInstruction(root, sc.DesignSpecRel) },
			Replanner: func() string { return buildAnalysisReplannerInstruction() },
		},
		workflow: basicWorkflow{
			baseTask: "基于仓库事实生成或修订设计文档：先读取 DESIGN_SPEC_PATH；若不存在则收集 README、docs、.env.example、清单与代表源码后写入。文档需覆盖项目概述、模块结构、环境要求、依赖安装、配置项与环境变量、启动/构建/测试命令，并在缺失时标注“仓库未提供”。文档需新增“需求分析支撑区”：技术栈矩阵、模块清单、已知约束、可观测性入口、可验收边界、需求待确认项。涉及数据库迁移时，须在仓库内核对 Flyway/Liquibase 等**实际加载路径**（见配置与目录），不得凭目录名臆测。",
			envelopeLines: []string{
				fmt.Sprintf("WORKSPACE_ROOT=%s", root),
				fmt.Sprintf("DESIGN_SPEC_PATH=%s", filepath.ToSlash(sc.DesignSpecRel)),
				fmt.Sprintf("DESIGN_SPEC_ABS=%s", sc.DesignSpecAbs),
			},
		},
		policy: toolkit.Policy{TempDirName: ".analysis-agent-tmp", AllowRunCommand: false, MissingPathAsExistsNo: true},
	}
}

func buildAnalysisPlannerInstruction(workspaceRoot, designSpecRel string) string {
	relSlash := filepath.ToSlash(designSpecRel)
	return fmt.Sprintf(`你是项目设计文档规划器。

## 目标
产出可落地的分析/更新计划。

## 规则
1) 将用户需求拆分为可执行步骤，产出可落地的分析/更新计划。
2) 规划必须基于工具读取的事实，禁止臆测未读过的文件内容。
3) 规划必须约束在工作区内：%s。
4) 设计文档目标路径（相对工作区）：%s。计划中必须包含：读取该路径（若存在）、收集 README、docs、.env.example、各语言清单与代表性源码；产出须便于他人上手：启动方式、配置与环境变量、依赖安装、构建/测试命令等；最后 write_file 与 read_file 校验。
5) 若项目使用 Flyway/Liquibase 等：计划中须包含「从 pom.xml/build.gradle、application*.yml 或代码中确认迁移脚本**被运行时加载的路径**（如 spring.flyway.locations、默认 classpath:db/migration）」的步骤，**禁止**未核对配置就把某目录（如 doc/sql）写成 Flyway 正式路径。
6) 计划中必须明确产出“需求分析支撑区”，至少包含：
   - tech_stack_matrix：前端/后端/数据库/基础设施；
   - module_catalog[]：模块名、职责、上下游依赖、用户可感知输入输出；
   - known_constraints[]：权限、事务、一致性、性能、安全、合规（缺失则写明未提供）；
   - observability_entrypoints[]：日志、监控、告警、审计入口；
   - unknowns_for_requirement[]：供 req 阶段向用户补充确认的问题。
7) 输出要可执行、尽量短小，避免无关步骤。`, workspaceRoot, relSlash)
}

func buildAnalysisExecutorInstruction(workspaceRoot, designSpecRel string) string {
	relSlash := filepath.ToSlash(designSpecRel)
	return fmt.Sprintf(`你是项目设计与架构文档维护 Agent。

## 规则
1) 先 list_dir / read_file 获取上下文，再修改文档，避免盲改。read_file 在目标不存在时返回 exists=false（非错误）；list_dir 在目录不存在时 exists=false、entries 为空（非错误）。
2) 只允许操作工作区：%s。
3) 主交付物为设计说明 Markdown，相对路径：%s（写入时使用相对 WORKSPACE_ROOT 的路径，例如 %s）。
4) 不允许只回复计划性套话，必须实际调用工具执行。
5) design 文档须包含（无则注明「仓库未提供」并说明已查路径）：项目概述；目录与模块；核心数据流或请求链路；环境与依赖（语言/运行时版本、包管理器）；安装与构建；配置与环境变量（可对照 .env.example 等）；如何启动（开发/生产）、默认端口或访问 URL；如何运行测试/代码检查；故障排查或常见问题（若有材料）。另保留：外部依赖、已知限制与待办。
5.1) 必须新增“需求分析支撑区”，并以可交接结构输出：
   - tech_stack_matrix；
   - module_catalog[]（模块名、职责、上下游、可验收边界）；
   - known_constraints[]；
   - observability_entrypoints[]；
   - unknowns_for_requirement[]。
6) **数据库迁移路径（强约束）**：若文档中描述 Flyway/Liquibase 或「迁移脚本位置」，必须先在工作区内用工具核对事实：
   - 查阅 pom.xml、build.gradle、application*.yml / application*.properties 中的 flyway、spring.flyway、liquibase 等配置；
   - 搜索 db/migration、classpath:db 等实际被引用的路径；
   - **运行时由 Flyway 加载的目录**须在「数据库初始化/迁移」小节写为**准确相对路径**（例如某模块 src/main/resources/db/migration），并说明依据（配置片段或依赖）；
   - 若仓库另有 doc/sql、根目录 sql 等仅作参考或手工执行的脚本，须与上述**区分表述**，**禁止**在目录树或说明中将其笼统标为「Flyway 迁移脚本」除非配置证明 Flyway 从该路径加载。
7) 若已有设计文档，优先在保留合理结构下修订补全上述「使用与运维」信息；若无则创建父目录并写入完整文档。
8) 临时笔记仅可写到 .analysis-agent-tmp（write_temp_file），不可与用户交付文件混放。
9) 完成 write_file 后必须 read_file 校验目标文件。
10) 最终输出须说明：相对 %s 的改动要点、补充的需求分析支撑字段、仍不确定之处（若有）。`, workspaceRoot, relSlash, relSlash, relSlash)
}

func buildAnalysisReplannerInstruction() string {
	return `你是重规划 Agent。

## 规则
1) 先判断任务是否已完成（设计文档已按事实更新并校验，且已覆盖或明确缺失项：环境、依赖安装、配置/环境变量、启动与构建/测试等上手信息；已包含“需求分析支撑区”（tech_stack_matrix、module_catalog、known_constraints、observability_entrypoints、unknowns_for_requirement）；若文中含数据库迁移，Flyway/同类工具路径已与配置一致、未误标 doc/sql 等为运行时路径）；若已完成，直接给出最终响应，不要继续新增步骤。
2) 只有在确实失败或目标未完成时，才进行重规划。
3) 重规划时优先最小改动，不推翻全部上下文。
4) 若无法继续，明确阻塞原因与下一步建议。`
}
