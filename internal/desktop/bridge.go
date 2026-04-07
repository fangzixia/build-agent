package desktop

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"build-agent/internal/config"
	"build-agent/internal/core"
	"build-agent/internal/toolkit"
)

// Bridge 提供前端可调用的 Go 方法
type Bridge struct {
	ctx context.Context
	cfg *config.Config
}

// NewBridge 创建新的 Bridge 实例
func NewBridge(cfg *config.Config) *Bridge {
	return &Bridge{cfg: cfg}
}

// Startup 在应用启动时调用
func (b *Bridge) Startup(ctx context.Context) {
	b.ctx = ctx
}

// Shutdown 在应用关闭时调用
func (b *Bridge) Shutdown(ctx context.Context) {
	// 清理资源
}

// RunTask 执行任务（暴露给前端）
func (b *Bridge) RunTask(agentName, task string) (*core.RunResult, error) {
	svc, err := core.NewService(b.ctx, b.cfg, agentName)
	if err != nil {
		return nil, err
	}
	return svc.RunTask(b.ctx, task)
}

// RunTaskWithProgress 执行任务并流式返回进度
func (b *Bridge) RunTaskWithProgress(agentName, task string) (*core.RunResult, error) {
	svc, err := core.NewService(b.ctx, b.cfg, agentName)
	if err != nil {
		return nil, err
	}

	// 使用 runtime.EventsEmit 发送进度事件到前端
	result, err := svc.RunTaskWithProgress(b.ctx, task, func(log core.EventLog) {
		runtime.EventsEmit(b.ctx, "task:progress", log)
	})

	return result, err
}

// GetConfig 获取配置信息（不包含敏感数据）
func (b *Bridge) GetConfig() (map[string]interface{}, error) {
	return map[string]interface{}{
		"workspaceRoot": b.cfg.Base.WorkspaceRoot,
		"httpAddr":      b.cfg.Base.HTTPAddr,
		"configPath":    b.getConfigFilePath(), // 添加配置文件路径
	}, nil
}

// GetEnvConfig 读取 .env 文件内容
func (b *Bridge) GetEnvConfig() (*config.EnvConfig, error) {
	// 获取配置文件路径（优先使用用户配置目录）
	envPath := b.getConfigFilePath()

	// 创建解析器
	parser := config.NewEnvParser()

	// 读取 .env 文件
	content, err := os.ReadFile(envPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，返回默认配置模板
			return b.getDefaultEnvConfig(), nil
		}
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// 解析配置
	envConfig, err := parser.Parse(string(content))
	if err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	return envConfig, nil
}

// getConfigFilePath 获取配置文件路径
func (b *Bridge) getConfigFilePath() string {
	// 使用用户主目录下的配置文件
	homeDir, err := os.UserHomeDir()
	if err != nil {
		// 如果获取失败，使用当前工作目录
		cwd, _ := os.Getwd()
		return filepath.Join(cwd, ".env")
	}

	// 在用户主目录下创建 .build-agent 目录
	configDir := filepath.Join(homeDir, ".build-agent")
	os.MkdirAll(configDir, 0755)

	return filepath.Join(configDir, "config.env")
}

// getDefaultEnvConfig 返回默认配置模板
func (b *Bridge) getDefaultEnvConfig() *config.EnvConfig {
	return &config.EnvConfig{
		Sections: map[string][]config.EnvEntry{
			"Base": {
				{Key: "WORKSPACE_ROOT", Value: "", Section: "Base", Comment: "项目工作目录"},
				{Key: "CMD_TIMEOUT_SEC", Value: "90", Section: "Base", Comment: "命令超时时间（秒）"},
				{Key: "HTTP_ADDR", Value: ":8080", Section: "Base", Comment: "HTTP 服务地址"},
			},
			"Code": {
				{Key: "CODE_OPENAI_BASE_URL", Value: "https://api.openai.com/v1", Section: "Code", Comment: "编码执行 - OpenAI API 地址"},
				{Key: "CODE_OPENAI_API_KEY", Value: "", Section: "Code", Comment: "编码执行 - OpenAI API Key"},
				{Key: "CODE_OPENAI_MODEL", Value: "gpt-4o-mini", Section: "Code", Comment: "编码执行 - 模型名称"},
				{Key: "CODE_EXECUTOR_MAX_ITERATIONS", Value: "100", Section: "Code", Comment: "编码执行 - 最大迭代次数"},
				{Key: "CODE_PLAN_EXECUTE_MAX_ITERATIONS", Value: "100", Section: "Code", Comment: "编码执行 - 计划执行最大迭代次数"},
			},
			"Build": {
				{Key: "BUILD_OPENAI_BASE_URL", Value: "https://api.openai.com/v1", Section: "Build", Comment: "完整构建 - OpenAI API 地址"},
				{Key: "BUILD_OPENAI_API_KEY", Value: "", Section: "Build", Comment: "完整构建 - OpenAI API Key"},
				{Key: "BUILD_OPENAI_MODEL", Value: "gpt-4o-mini", Section: "Build", Comment: "完整构建 - 模型名称"},
				{Key: "BUILD_EXECUTOR_MAX_ITERATIONS", Value: "20", Section: "Build", Comment: "完整构建 - 最大迭代次数"},
				{Key: "BUILD_PLAN_EXECUTE_MAX_ITERATIONS", Value: "10", Section: "Build", Comment: "完整构建 - 计划执行最大迭代次数"},
				{Key: "BUILD_MAX_RETRIES", Value: "5", Section: "Build", Comment: "完整构建 - 最大重试次数"},
			},
			"Analysis": {
				{Key: "ANALYSIS_OPENAI_BASE_URL", Value: "https://api.openai.com/v1", Section: "Analysis", Comment: "项目分析 - OpenAI API 地址"},
				{Key: "ANALYSIS_OPENAI_API_KEY", Value: "", Section: "Analysis", Comment: "项目分析 - OpenAI API Key"},
				{Key: "ANALYSIS_OPENAI_MODEL", Value: "gpt-4o-mini", Section: "Analysis", Comment: "项目分析 - 模型名称"},
				{Key: "ANALYSIS_EXECUTOR_MAX_ITERATIONS", Value: "100", Section: "Analysis", Comment: "项目分析 - 最大迭代次数"},
				{Key: "ANALYSIS_PLAN_EXECUTE_MAX_ITERATIONS", Value: "100", Section: "Analysis", Comment: "项目分析 - 计划执行最大迭代次数"},
			},
			"Eval": {
				{Key: "EVAL_OPENAI_BASE_URL", Value: "https://api.openai.com/v1", Section: "Eval", Comment: "验收评测 - OpenAI API 地址"},
				{Key: "EVAL_OPENAI_API_KEY", Value: "", Section: "Eval", Comment: "验收评测 - OpenAI API Key"},
				{Key: "EVAL_OPENAI_MODEL", Value: "gpt-4o-mini", Section: "Eval", Comment: "验收评测 - 模型名称"},
				{Key: "EVAL_EXECUTOR_MAX_ITERATIONS", Value: "100", Section: "Eval", Comment: "验收评测 - 最大迭代次数"},
				{Key: "EVAL_PLAN_EXECUTE_MAX_ITERATIONS", Value: "100", Section: "Eval", Comment: "验收评测 - 计划执行最大迭代次数"},
				{Key: "EVAL_PASS_SCORE_THRESHOLD", Value: "80", Section: "Eval", Comment: "验收评测 - 通过分数阈值"},
			},
			"Requirements": {
				{Key: "REQUIREMENTS_OPENAI_BASE_URL", Value: "https://api.openai.com/v1", Section: "Requirements", Comment: "需求分析 - OpenAI API 地址"},
				{Key: "REQUIREMENTS_OPENAI_API_KEY", Value: "", Section: "Requirements", Comment: "需求分析 - OpenAI API Key"},
				{Key: "REQUIREMENTS_OPENAI_MODEL", Value: "gpt-4o-mini", Section: "Requirements", Comment: "需求分析 - 模型名称"},
				{Key: "REQUIREMENTS_EXECUTOR_MAX_ITERATIONS", Value: "100", Section: "Requirements", Comment: "需求分析 - 最大迭代次数"},
				{Key: "REQUIREMENTS_PLAN_EXECUTE_MAX_ITERATIONS", Value: "100", Section: "Requirements", Comment: "需求分析 - 计划执行最大迭代次数"},
			},
		},
	}
}

// SaveEnvConfig 保存 .env 文件
func (b *Bridge) SaveEnvConfig(envConfig *config.EnvConfig) error {
	// 获取配置文件路径
	envPath := b.getConfigFilePath()

	// 创建解析器
	parser := config.NewEnvParser()

	// 序列化配置
	content, err := parser.Serialize(envConfig)
	if err != nil {
		return fmt.Errorf("failed to serialize config: %w", err)
	}

	// 原子性写入文件
	tmpPath := envPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// 验证写入成功后重命名
	if err := os.Rename(tmpPath, envPath); err != nil {
		// 清理临时文件
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	// 保存成功后，重新加载配置到环境变量
	if err := b.reloadConfig(envPath); err != nil {
		return fmt.Errorf("failed to reload config: %w", err)
	}

	return nil
}

// reloadConfig 重新加载配置到环境变量
func (b *Bridge) reloadConfig(envPath string) error {
	// 读取配置文件
	content, err := os.ReadFile(envPath)
	if err != nil {
		return err
	}

	// 解析并设置环境变量
	lines := strings.Split(string(content), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			key := strings.TrimSpace(parts[0])
			value := strings.TrimSpace(parts[1])
			os.Setenv(key, value)
		}
	}

	// 重新加载应用配置
	newCfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("failed to reload application config: %w", err)
	}

	// 更新 bridge 的配置
	b.cfg = newCfg

	return nil
}

// GetEnvConfigExample 读取 .env.example 文件
// ReadFile 读取工作目录内的文件
func (b *Bridge) ReadFile(path string) (string, error) {
	// 使用 toolkit.resolveSafePath 验证路径
	safePath, err := toolkit.ResolveSafePath(b.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}

	// 读取文件内容
	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}

	return string(content), nil
}

// SaveFile 保存文件到工作目录
func (b *Bridge) SaveFile(path, content string) error {
	// 使用 toolkit.resolveSafePath 验证路径
	safePath, err := toolkit.ResolveSafePath(b.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}

	// 确保目录存在
	dir := filepath.Dir(safePath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}

	// 写入文件
	if err := os.WriteFile(safePath, []byte(content), 0644); err != nil {
		return fmt.Errorf("write file: %w", err)
	}

	return nil
}

// FileInfo 文件信息结构
type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// ListFiles 列出目录内容
func (b *Bridge) ListFiles(path string) ([]FileInfo, error) {
	// 使用 toolkit.resolveSafePath 验证路径
	safePath, err := toolkit.ResolveSafePath(b.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}

	// 读取目录内容
	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}

	// 转换为 FileInfo 列表
	result := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue // 跳过无法获取信息的文件
		}

		result = append(result, FileInfo{
			Name:  entry.Name(),
			IsDir: entry.IsDir(),
			Size:  info.Size(),
		})
	}

	return result, nil
}
