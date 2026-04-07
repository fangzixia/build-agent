# Build Agent

**AI 驱动的软件开发闭环工具**

通过 4 个专业 Agent 的协作，实现从需求分析到代码实现、再到验收评测的完整开发闭环。

## ✨ 特色功能

- 🖥️ **双模式运行**：支持桌面应用和 Web 服务器两种模式
- 🤖 **4 个专业 Agent**：分析、需求、编码、评测全流程覆盖
- 🔄 **完整闭环**：自动化从需求到交付的全过程
- 📊 **实时监控**：流式输出执行日志，实时查看进度
- 🎨 **现代 UI**：直观的 Web 界面，支持文件编辑和预览
- 🔧 **灵活配置**：每个 Agent 独立配置，支持多种 LLM 服务

## 快速开始

### 1. 环境准备

**基础要求：**
- Go >= 1.24
- 配置 OpenAI API（或兼容的 LLM 服务）

**桌面模式额外要求：**
- Wails CLI：`go install github.com/wailsapp/wails/v2/cmd/wails@latest`
- 系统依赖：
  - Windows: WebView2 (通常已预装)
  - macOS: 无需额外依赖
  - Linux: `webkit2gtk` 和 `gtk3`

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

### 3. 启动方式

#### 方式 1：桌面应用模式（推荐）

```bash
# 开发模式（支持热重载）
wails dev

# 或直接运行
go run ./cmd/desktop

# 生产构建
wails build
```

**桌面模式特性：**
- ✅ 原生窗口体验
- ✅ 系统托盘集成（最小化到托盘）
- ✅ 无需浏览器
- ✅ 更快的响应速度
- ✅ 跨平台支持（Windows、macOS、Linux）

#### 方式 2：Web 服务器模式

```bash
# 启动 HTTP 服务器
go run ./cmd/agent serve --addr :8080

# 在浏览器中访问
# http://localhost:8080
```

#### 方式 3：命令行模式

```bash
# 直接执行任务
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

### 桌面应用（推荐）

启动桌面应用后，享受原生窗口体验：

**主要功能：**
- **仪表盘**：查看需求统计、通过率、失败项分析
- **需求管理**：创建、查看、编辑需求文档
- **执行任务**：一键执行分析、创建需求、编码、评测
- **验收历史**：查看所有评测记录和详情
- **系统配置**：直接编辑 .env 配置文件
- **文件编辑器**：Markdown 编辑、实时预览、分屏模式

**系统托盘功能：**
- 显示/隐藏窗口
- 后台运行
- 快速退出

**快捷操作：**
- 关闭窗口 → 最小化到托盘（不退出应用）
- 托盘图标右键 → 显示菜单
- 系统托盘 → 退出 → 完全关闭应用

### Web UI

启动服务后访问 http://localhost:8080

功能与桌面应用完全一致，适合：
- 远程访问场景
- 多用户协作
- 无需安装客户端

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

- **双模式运行**：桌面应用 + Web 服务器，灵活选择
- **单一二进制**：所有功能集成在一个可执行文件
- **配置隔离**：每个 Agent 使用独立的 LLM 配置
- **Plan-Execute**：智能规划和执行任务
- **流式输出**：实时查看执行日志
- **文件管理**：自动管理需求和评测文档
- **完整闭环**：从需求到交付的自动化流程
- **系统托盘**：桌面模式支持后台运行
- **跨平台**：支持 Windows、macOS、Linux

## 项目结构

```
build-agent/
├── cmd/
│   └── agent/          # CLI 入口（服务器模式）
├── internal/
│   ├── agents/         # 4 个 Agent 实现
│   ├── core/           # Plan-Execute 核心
│   ├── http/           # HTTP 服务和 API
│   ├── wails/          # Wails 桌面应用层
│   │   ├── bridge.go   # Go-JavaScript 桥接
│   │   ├── tray.go     # 系统托盘管理
│   │   └── errors.go   # 错误处理
│   ├── config/         # 配置管理
│   └── toolkit/        # 工具集（文件、命令等）
├── frontend/           # 前端资源（桌面和服务器共享）
│   ├── index.html
│   └── static/
│       ├── css/
│       └── js/
│           ├── app.js          # 主应用逻辑
│           └── api-adapter.js  # 双模式适配层
├── build/              # 桌面应用资源
│   └── appicon.png     # 应用图标
├── .spec/              # 需求和评测文档（自动生成）
│   ├── design.md       # 项目设计文档
│   ├── REQ-xxxxx.md    # 需求文档
│   └── EVAL-*.md       # 评测报告
├── main.go             # 桌面应用入口
├── wails.json          # Wails 配置
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

### 场景 2：使用桌面应用

1. 启动桌面应用（`wails dev` 或运行构建的可执行文件）
2. 在"需求管理"标签页点击"创建需求"，输入需求描述
3. 在"执行任务"标签页选择"编码实现"，选择需求文件
4. 点击"验收评测"，查看评测结果
5. 如果未通过，重复步骤 3-4
6. 关闭窗口会最小化到系统托盘，应用继续在后台运行

### 场景 3：使用 Web UI

1. 启动服务：`go run ./cmd/agent serve --addr :8080`
2. 访问 http://localhost:8080
3. 点击"创建需求"，输入需求描述
4. 点击"编码实现"，选择需求文件
5. 点击"验收评测"，查看评测结果
6. 如果未通过，重复步骤 4-5

### 场景 4：完整构建（自动循环）

```bash
# 一次性完成：编码 → 评测 → 修复 → 重新评测，直到通过
go run ./cmd/agent build run --task "实现并验收用户登录功能"
```

## 构建与部署

### 桌面应用构建

```bash
# 开发模式（支持热重载）
wails dev

# 生产构建（当前平台）
wails build

# 指定平台构建
wails build -platform windows/amd64
wails build -platform darwin/amd64
wails build -platform linux/amd64

# 使用构建脚本
# Windows
.\scripts\build-desktop.bat

# Linux/macOS
./scripts/build-desktop.sh
```

构建产物位于 `build/bin/` 目录。

### Web 服务器部署

```bash
# 编译二进制
go build -o build-agent ./cmd/agent

# 启动服务
./build-agent serve --addr :8080

# 使用 systemd（Linux）
# 创建 /etc/systemd/system/build-agent.service
[Unit]
Description=Build Agent Service
After=network.target

[Service]
Type=simple
User=your-user
WorkingDirectory=/path/to/build-agent
ExecStart=/path/to/build-agent serve --addr :8080
Restart=on-failure

[Install]
WantedBy=multi-user.target
```

### Docker 部署

```dockerfile
FROM golang:1.24-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o build-agent ./cmd/agent

FROM alpine:latest
RUN apk --no-cache add ca-certificates
WORKDIR /root/
COPY --from=builder /app/build-agent .
COPY --from=builder /app/web ./web
COPY .env.example .env
EXPOSE 8080
CMD ["./build-agent", "serve", "--addr", ":8080"]
```

## 开发与扩展

### 添加新 Agent

1. 在 `internal/agents/` 创建新的 Agent 文件
2. 实现 `Agent` 接口
3. 在 `internal/cli/` 添加命令
4. 在 `internal/http/` 添加 API 路由
5. 在 `internal/wails/bridge.go` 添加桌面应用支持（可选）

### 自定义工具

在 `internal/toolkit/` 中添加新的工具函数，所有 Agent 都可以使用。

### 双模式架构说明

Build Agent 采用统一的前后端架构，支持桌面和 Web 两种运行模式：

**前端层（frontend/）**：
- `index.html` 和 `static/` 目录被嵌入到二进制文件中
- `wails-adapter.js` 提供统一的 API 适配层
- 自动检测运行环境（Wails 或 HTTP）并切换通信方式

**后端层**：
- 桌面模式：通过 `internal/wails/bridge.go` 提供 Go-JavaScript 桥接
- Web 模式：通过 `internal/http/` 提供 RESTful API
- 两种模式共享相同的业务逻辑（`internal/agents/` 和 `internal/core/`）

**优势**：
- 一套代码，两种部署方式
- 前端无需关心运行环境
- 后端逻辑完全复用
- 灵活选择适合的运行模式

## 常见问题

### 通用问题

**Q: 如何使用不同的 LLM 服务？**  
A: 修改 `.env` 中的 `*_OPENAI_BASE_URL`，指向兼容 OpenAI API 的服务（如 Azure OpenAI、本地模型等）。

**Q: 需求文件编号如何管理？**  
A: 自动按 `REQ-00001.md`、`REQ-00002.md` 递增，无需手动管理。

**Q: 如何修改已有需求？**  
A: 在任务中指定文件名，如 `"修改 REQ-00001.md，补充验收标准"`。

**Q: 评测不通过怎么办？**  
A: 查看 `EVAL-*.md` 中的失败项和改进建议，修复后重新评测。

### 桌面应用问题

**Q: 桌面应用和 Web 模式有什么区别？**  
A: 桌面应用提供原生窗口体验、系统托盘集成、更快的响应速度，无需浏览器。Web 模式适合远程访问和多用户协作。两者功能完全一致。

**Q: 如何在桌面模式下编辑配置？**  
A: 在桌面应用中点击"系统配置"标签页，可以直接编辑 `.env` 文件并保存。

**Q: 桌面应用关闭后如何重新打开？**  
A: 关闭窗口会最小化到系统托盘，点击托盘图标可重新显示。要完全退出，请右键托盘图标选择"退出"。

**Q: 桌面应用构建失败怎么办？**  
A: 确保已安装 Wails CLI 和系统依赖。Windows 需要 WebView2，Linux 需要 `webkit2gtk` 和 `gtk3`。详见 [Wails 官方文档](https://wails.io/docs/gettingstarted/installation)。

**Q: 如何跨平台构建桌面应用？**  
A: 使用 `wails build` 构建当前平台，或使用 `wails build -platform windows/amd64` 指定目标平台。详见构建脚本部分。

## 许可证

本项目未指定许可证，请根据实际情况添加。

## 相关文档

- [Web UI 功能说明](WEB_UI_NEW_FEATURES.md)
- [系统评估报告](SYSTEM_ASSESSMENT.md)
- [改进建议](IMPROVEMENT_RECOMMENDATIONS.md)
