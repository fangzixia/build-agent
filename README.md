# Build Agent

**AI 驱动的软件开发闭环工具**

通过 4 个专业 Agent 的协作，实现从需求分析到代码实现、再到验收评测的完整开发闭环。

## 快速开始

### 1. 环境准备

- Go >= 1.24
- 配置 OpenAI API（或兼容的 LLM 服务）

### 2. 配置

```bash
# 复制配置文件
cp .env.example .env

# 编辑 .env，填入你的 API 配置
# 每个 Agent 使用独立配置：
# - ANALYSIS_OPENAI_API_KEY / ANALYSIS_OPENAI_BASE_URL
# - REQUIREMENTS_OPENAI_API_KEY / REQUIREMENTS_OPENAI_BASE_URL  
# - CODE_OPENAI_API_KEY / CODE_OPENAI_BASE_URL
# - EVAL_OPENAI_API_KEY / EVAL_OPENAI_BASE_URL
```

### 3. 启动

```bash
# 安装依赖
go mod tidy

# 启动 Web UI（推荐）
go run ./cmd/agent serve --addr :8080
# 访问 http://localhost:8080

# 或使用命令行
go run ./cmd/agent req run --task "用户登录功能需求"
go run ./cmd/agent code run --task "实现登录功能"
go run ./cmd/agent eval run --task "验收登录功能"
```

## 核心功能

### 4 个专业 Agent

| Agent | 功能 | 输出 |
|-------|------|------|
| **Analysis** | 分析项目结构、技术栈、API 契约 | `.spec/design.md` |
| **Requirements** | 生成需求文档和验收标准（用户视角） | `.spec/REQ-xxxxx.md` |
| **Code** | 根据需求实现/修改代码 | 源代码文件 |
| **Eval** | 验收评测，生成评分和改进建议 | `.spec/EVAL-REQ-xxxxx-xx.md` |

### 开发闭环

```
需求 → 分析 → 编码 → 评测 → (未通过则修复) → 通过
```

## 使用方式

### Web UI（推荐）

启动服务后访问 http://localhost:8080

- **仪表盘**：查看需求统计、通过率、失败项分析
- **需求管理**：创建、查看、编辑需求文档
- **执行任务**：一键执行分析、创建需求、编码、评测
- **验收历史**：查看所有评测记录和详情
- **文件编辑器**：Markdown 编辑、实时预览、分屏模式

### 命令行

```bash
# 分析项目
go run ./cmd/agent analysis run --task "分析项目结构"

# 创建需求
go run ./cmd/agent req run --task "用户登录功能需求"

# 编码实现
go run ./cmd/agent code run --task "实现登录功能"

# 验收评测
go run ./cmd/agent eval run --task "验收登录功能"

# 完整构建（编码→评测循环直到通过）
go run ./cmd/agent build run --task "实现并验收登录功能"

# Chat 模式（多轮交互）
go run ./cmd/agent req chat
```

### HTTP API

```bash
# 启动服务
go run ./cmd/agent serve --addr :8080

# 调用 API
curl -X POST http://localhost:8080/v1/req/run \
  -H "Content-Type: application/json" \
  -d '{"task":"用户登录功能需求"}'

# 流式输出（实时日志）
curl -X POST http://localhost:8080/v1/code/run \
  -H "Content-Type: application/json" \
  -H "Accept: text/event-stream" \
  -d '{"task":"实现登录功能"}'
```

## 核心特性

- **单一二进制**：所有功能集成在一个可执行文件
- **配置隔离**：每个 Agent 使用独立的 LLM 配置
- **Plan-Execute**：智能规划和执行任务
- **流式输出**：实时查看执行日志
- **文件管理**：自动管理需求和评测文档
- **完整闭环**：从需求到交付的自动化流程

## 项目结构

```
build-agent/
├── cmd/agent/          # CLI 入口
├── internal/
│   ├── agents/         # 4 个 Agent 实现
│   ├── core/           # Plan-Execute 核心
│   ├── http/           # HTTP 服务和 API
│   ├── config/         # 配置管理
│   └── toolkit/        # 工具集（文件、命令等）
├── web/                # Web UI 前端
│   ├── index.html
│   └── static/
├── .spec/              # 需求和评测文档（自动生成）
│   ├── design.md       # 项目设计文档
│   ├── REQ-xxxxx.md    # 需求文档
│   └── EVAL-*.md       # 评测报告
└── .env                # 配置文件
```

## 配置说明

每个 Agent 使用独立的环境变量配置：

```bash
# Analysis Agent
ANALYSIS_OPENAI_API_KEY=sk-xxx
ANALYSIS_OPENAI_BASE_URL=https://api.openai.com/v1
ANALYSIS_OPENAI_MODEL=gpt-4
ANALYSIS_DESIGN_SPEC_PATH=.spec/design.md

# Requirements Agent
REQUIREMENTS_OPENAI_API_KEY=sk-xxx
REQUIREMENTS_OPENAI_BASE_URL=https://api.openai.com/v1
REQUIREMENTS_OPENAI_MODEL=gpt-4
REQUIREMENTS_SPEC_DIR=.spec

# Code Agent
CODE_OPENAI_API_KEY=sk-xxx
CODE_OPENAI_BASE_URL=https://api.openai.com/v1
CODE_OPENAI_MODEL=gpt-4

# Eval Agent
EVAL_OPENAI_API_KEY=sk-xxx
EVAL_OPENAI_BASE_URL=https://api.openai.com/v1
EVAL_OPENAI_MODEL=gpt-4
```

详细配置请参考 `.env.example`。

## 典型场景

### 场景 1：新功能开发

```bash
# 1. 创建需求
go run ./cmd/agent req run --task "用户注册功能：邮箱注册、密码强度验证、邮箱验证"

# 2. 实现代码
go run ./cmd/agent code run --task "实现 REQ-00001.md 中的注册功能"

# 3. 验收评测
go run ./cmd/agent eval run --task "验收注册功能"

# 4. 如果未通过，修复后重新评测
go run ./cmd/agent code run --task "修复 EVAL-REQ-00001-01.md 中的问题"
go run ./cmd/agent eval run --task "重新验收注册功能"
```

### 场景 2：使用 Web UI

1. 访问 http://localhost:8080
2. 点击"创建需求"，输入需求描述
3. 点击"编码实现"，选择需求文件
4. 点击"验收评测"，查看评测结果
5. 如果未通过，重复步骤 3-4

### 场景 3：完整构建（自动循环）

```bash
# 一次性完成：编码 → 评测 → 修复 → 重新评测，直到通过
go run ./cmd/agent build run --task "实现并验收用户登录功能"
```

## 开发与扩展

### 添加新 Agent

1. 在 `internal/agents/` 创建新的 Agent 文件
2. 实现 `Agent` 接口
3. 在 `internal/cli/` 添加命令
4. 在 `internal/http/` 添加 API 路由

### 自定义工具

在 `internal/toolkit/` 中添加新的工具函数，所有 Agent 都可以使用。

## 常见问题

**Q: 如何使用不同的 LLM 服务？**  
A: 修改 `.env` 中的 `*_OPENAI_BASE_URL`，指向兼容 OpenAI API 的服务（如 Azure OpenAI、本地模型等）。

**Q: 需求文件编号如何管理？**  
A: 自动按 `REQ-00001.md`、`REQ-00002.md` 递增，无需手动管理。

**Q: 如何修改已有需求？**  
A: 在任务中指定文件名，如 `"修改 REQ-00001.md，补充验收标准"`。

**Q: 评测不通过怎么办？**  
A: 查看 `EVAL-*.md` 中的失败项和改进建议，修复后重新评测。

## 许可证

本项目未指定许可证，请根据实际情况添加。

## 相关文档

- [Web UI 功能说明](WEB_UI_NEW_FEATURES.md)
- [系统评估报告](SYSTEM_ASSESSMENT.md)
- [改进建议](IMPROVEMENT_RECOMMENDATIONS.md)
