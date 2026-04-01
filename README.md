## build-agent

统一的多 Agent 编排服务，整合 `code`、`analysis`、`eval`、`requirements (req)` 四类能力，在**单一二进制**中提供多场景调用，并通过配置实现**能力强隔离**。

### 核心能力

- **多场景单二进制**：通过 `agent code|analysis|eval|req run/chat/serve` 统一入口完成所有能力调用  
- **公共 core**：基于 Plan-Execute 的装配流程、统一的事件流与轮次日志
- **公共 toolkit**：抽象文件工具、命令工具与策略隔离，方便扩展新能力
- **配置强隔离**：每个场景使用独立的 OPENAI 配置，不回退到全局 `OPENAI_*`
- **eval 场景增强**：支持前台执行 `python *.py`，自动切换为后台服务（日志写入 `.eval-agent-tmp/eval-server.log`）
- **req 场景产物管理**：
  - 自动生成 `.spec/REQ-xxxxx.md`，按五位流水号递增
  - 支持“指定文件修改”：任务中给出目标需求 md 文件名时，仅修改该文件

---

## 环境准备

- **语言环境**：Go \>= 1.24（`go.mod` 中为 `go 1.24.1`）
- **依赖管理**：使用 Go Modules（无需 `GOPATH`）
- **主要依赖**：
  - `github.com/cloudwego/eino` 及 `eino-ext`：用于 LLM 能力编排
  - `github.com/spf13/cobra`：CLI 命令行框架
  - `github.com/joho/godotenv`：加载 `.env` 配置

### 安装依赖

```bash
go mod tidy
```

---

## 配置说明

项目使用 `.env` / `.env.example` 管理敏感配置（如 OpenAI Key、BaseURL 等）。

1. 复制示例配置：

```bash
cp .env.example .env
```

2. 根据实际环境填入：

- **全局**：如需要，可配置公共 `OPENAI_*`（但各场景不会回退到这些全局变量）
- **按场景隔离**（示例命名，具体以代码实现为准）：
  - `CODE_OPENAI_API_KEY` / `CODE_OPENAI_BASE_URL`
  - `ANALYSIS_OPENAI_API_KEY` / `ANALYSIS_OPENAI_BASE_URL`
  - `EVAL_OPENAI_API_KEY` / `EVAL_OPENAI_BASE_URL`
  - `REQ_OPENAI_API_KEY` / `REQ_OPENAI_BASE_URL`

各场景缺失自己的配置时，不会自动回退到全局 `OPENAI_*`，以保证行为可预期。

---

## 目录结构概览

（仅列出与使用/二次开发关系最紧密的部分）

- `cmd/agent/main.go`：CLI 入口，定义 `agent` 主命令
- `internal/cli`：命令行参数与子命令（`code` / `analysis` / `eval` / `req` 等）
- `internal/agents`：各场景 Agent 实现
  - `code_agent.go`：代码修改/生成等能力
  - `analysis_agent.go`：代码/需求/文档分析能力
  - `eval_agent.go`：评测与执行相关能力
  - `requirements_agent.go`：需求文档生成与增量更新
  - `build.go` / `types.go`：Agent 构建与公共类型
- `internal/core`：核心调度与服务层
- `internal/http`：HTTP Server 及路由
- `internal/config`：配置加载与解析
- `internal/toolkit`：文件操作、命令执行、路径守卫等通用工具

---

## 命令行使用

### 基础运行

```bash
# 安装依赖
go mod tidy

# Code 场景：一次性任务
go run ./cmd/agent code run --task "你的任务"

# Analysis 场景
go run ./cmd/agent analysis run --task "请分析当前项目结构"

# Eval 场景
go run ./cmd/agent eval run --task "对 tests 目录下的用例进行评估"

# Req 场景：生成/补全需求
go run ./cmd/agent req run --task "用户登录功能的完整需求与验收标准"
go run ./cmd/agent req run --task "请更新 REQ-00001.md，补全登录需求验收标准"
```

### Chat 模式

适合与某一场景进行多轮交互：

```bash
go run ./cmd/agent code chat
go run ./cmd/agent analysis chat
go run ./cmd/agent eval chat
go run ./cmd/agent req chat
```

### 独立 Serve 模式（按场景暴露 HTTP）

每个场景可以单独开启 HTTP 服务：

```bash
go run ./cmd/agent code serve --addr :8080
go run ./cmd/agent analysis serve --addr :8081
go run ./cmd/agent eval serve --addr :8082
go run ./cmd/agent req serve --addr :8083
```

---

## HTTP 服务

项目同时提供一个统一的 HTTP 入口，方便通过 REST 接入：

```bash
go run ./cmd/agent serve --addr :8080
```

### 请求示例

```bash
# code 场景：一次性任务
curl -X POST http://localhost:8080/v1/code/run \
  -H "Content-Type: application/json" \
  -d "{\"task\":\"读取当前目录并总结\"}"

# analysis 场景
curl -X POST http://localhost:8080/v1/analysis/run \
  -H \"Content-Type: application/json\" \
  -d \"{\\\"task\\\":\\\"请分析当前代码架构\\\"}\"

# eval 场景
curl -X POST http://localhost:8080/v1/eval/run \
  -H \"Content-Type: application/json\" \
  -d \"{\\\"task\\\":\\\"对脚本运行结果进行评估\\\"}\"

# req 场景：生成需求
curl -X POST http://localhost:8080/v1/req/run \
  -H "Content-Type: application/json" \
  -d "{\"task\":\"请补全一个用户登录需求\"}"

# req 场景：指定需求文件增量修改
curl -X POST http://localhost:8080/v1/req/run \
  -H "Content-Type: application/json" \
  -d "{\"task\":\"请修改 REQ-00001.md，补全接口验收标准\"}"
```

### 兼容入口说明

当使用 `agent <scene> serve` 启动时，服务会为该场景提供一个兼容路径：

- 若使用 `agent analysis serve` 启动，则 `/v1/run` 会按 **analysis 场景** 执行。

---

## 常见使用场景示例

- **代码修改/实现功能（code）**  
  - 从自然语言任务生成/修改代码  
  - 对现有代码进行重构与补充注释（按策略约束）

- **架构/需求分析（analysis）**  
  - 总结项目目录与模块关系  
  - 从代码/文档中抽取需求与风险点

- **评测与执行（eval）**  
  - 运行指定 `python` 脚本并记录日志  
  - 针对用例/脚本输出自动生成评测结论

- **需求文档管理（req）**  
  - 生成全新需求说明书（`.spec/REQ-xxxxx.md`）  
  - 在指定需求文件上进行增量更新（如补充验收标准）

---

## 开发与扩展建议

- **新增场景**：在 `internal/agents` 中增加新的 Agent，并在 `internal/cli` / `internal/http` 中挂接对应命令和路由
- **复用工具**：优先使用 `internal/toolkit` 已有的文件与命令工具，以保证行为一致性与安全性
- **配置约束**：为新场景定义独立的环境变量前缀，延续“配置强隔离”的设计

---

## 许可证

当前仓库未在根目录提供显式许可证文件，如有需要可根据实际开源策略补充 `LICENSE`。
