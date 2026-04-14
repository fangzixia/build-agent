# Build Agent

AI 驱动的软件开发闭环桌面工具，通过多个专业 Agent 的协作，实现从需求分析到代码实现、再到验收评测的完整开发闭环。

## Agent 说明

| Agent | 功能 | 输出 |
|-------|------|------|
| Analysis | 分析项目结构、技术栈、API 契约 | `.spec/DESIGN.md` |
| Requirements | 生成需求文档和验收标准 | `.spec/REQ-xxxxx.md` |
| Code | 根据需求实现/修改代码 | 源代码文件 |
| Eval | 验收评测，生成评分和改进建议 | `.spec/EVAL-REQ-xxxxx-xx.md` |
| Build | 编码 → 评测循环，直到通过 | — |

## 构建

依赖：Go >= 1.21、[Wails CLI](https://wails.io/docs/gettingstarted/installation)

```bash
# 开发模式
wails dev

# 生产构建
wails build
# 或
./scripts/build-desktop.sh   # Linux/macOS
scripts\build-desktop.bat    # Windows
```

构建产物在 `build/bin/` 目录。

## 配置

首次启动后在「系统配置」页面填写：

```
OPENAI_BASE_URL=https://api.openai.com/v1
OPENAI_API_KEY=your_api_key
OPENAI_MODEL=gpt-4o-mini
```

配置文件存储位置：
- Windows: `%APPDATA%\build-agent\config.env`
- macOS: `~/Library/Application Support/build-agent/config.env`
- Linux: `~/.config/build-agent/config.env`

工作区选择和最近记录存储在同目录的 `workspaces.json`。
