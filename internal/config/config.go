package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

type BaseConfig struct {
	WorkspaceRoot   string
	CmdTimeoutSec   int
	HTTPAddr        string
	BuildMaxRetries int
}

type AgentConfig struct {
	Name                     string
	OpenAIBaseURL            string
	OpenAIAPIKey             string
	OpenAIModel              string
	ExecutorMaxIterations    int
	PlanExecuteMaxIterations int
	DesignSpecRel            string
	DesignSpecAbs            string
	RequirementsSpecRel      string
	RequirementsSpecAbs      string
	RequirementsSpecDirRel   string
	RequirementsSpecDirAbs   string
}

type DesktopConfig struct {
	// 窗口配置
	WindowTitle  string
	WindowWidth  int
	WindowHeight int
	MinWidth     int
	MinHeight    int

	// 系统托盘配置
	EnableTray bool
	TrayIcon   string

	// 开发模式配置
	DevMode      bool
	DevServerURL string
}

type Config struct {
	Base    BaseConfig
	Agent   map[string]AgentConfig
	Desktop DesktopConfig
}

func Load() (*Config, error) {
	// .env 文件是可选的，不存在也不影响启动
	_ = godotenv.Load(".env")

	c := &Config{
		Base: BaseConfig{
			WorkspaceRoot:   os.Getenv("WORKSPACE_ROOT"),
			CmdTimeoutSec:   getEnvInt("CMD_TIMEOUT_SEC", 90),
			HTTPAddr:        getEnv("HTTP_ADDR", ":8080"),
			BuildMaxRetries: getEnvInt("BUILD_MAX_RETRIES", 5),
		},
		Agent: make(map[string]AgentConfig, 5),
		Desktop: DesktopConfig{
			WindowTitle:  getEnv("DESKTOP_WINDOW_TITLE", "Build Agent"),
			WindowWidth:  getEnvInt("DESKTOP_WINDOW_WIDTH", 1280),
			WindowHeight: getEnvInt("DESKTOP_WINDOW_HEIGHT", 800),
			MinWidth:     getEnvInt("DESKTOP_MIN_WIDTH", 800),
			MinHeight:    getEnvInt("DESKTOP_MIN_HEIGHT", 600),
			EnableTray:   getEnvBool("DESKTOP_ENABLE_TRAY", true),
			TrayIcon:     getEnv("DESKTOP_TRAY_ICON", ""),
			DevMode:      getEnvBool("DESKTOP_DEV_MODE", false),
			DevServerURL: getEnv("DESKTOP_DEV_SERVER_URL", ""),
		},
	}
	if c.Base.WorkspaceRoot == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, fmt.Errorf("get working directory: %w", err)
		}
		c.Base.WorkspaceRoot = cwd
	}
	absRoot, err := filepath.Abs(c.Base.WorkspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve WORKSPACE_ROOT: %w", err)
	}
	c.Base.WorkspaceRoot = absRoot

	// 加载各个 agent 配置，使用默认值
	c.Agent["code"], err = loadAgent("CODE", c.Base.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	c.Agent["analysis"], err = loadAgent("ANALYSIS", c.Base.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	c.Agent["eval"], err = loadAgent("EVAL", c.Base.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	c.Agent["requirements"], err = loadAgent("REQUIREMENTS", c.Base.WorkspaceRoot)
	if err != nil {
		return nil, err
	}
	c.Agent["build"], err = loadAgent("BUILD", c.Base.WorkspaceRoot)
	if err != nil {
		return nil, err
	}

	// 验证配置（如果没有配置 OpenAI，给出警告而不是错误）
	return c, c.ValidateWithWarnings()
}

func loadAgent(prefix, root string) (AgentConfig, error) {
	sc := AgentConfig{
		Name:                     strings.ToLower(prefix),
		OpenAIBaseURL:            getEnv(prefix+"_OPENAI_BASE_URL", "https://api.openai.com/v1"),
		OpenAIAPIKey:             getEnv(prefix+"_OPENAI_API_KEY", ""),
		OpenAIModel:              getEnv(prefix+"_OPENAI_MODEL", "gpt-4o-mini"),
		ExecutorMaxIterations:    getEnvInt(prefix+"_EXECUTOR_MAX_ITERATIONS", 100),
		PlanExecuteMaxIterations: getEnvInt(prefix+"_PLAN_EXECUTE_MAX_ITERATIONS", 100),
	}
	// 使用默认的 design spec 路径
	designRaw := strings.TrimSpace(getEnv(prefix+"_DESIGN_SPEC_PATH", ".spec/design.md"))
	absDesign, relDesign, err := resolvePathUnderRoot(root, designRaw)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("%s_DESIGN_SPEC_PATH: %w", prefix, err)
	}
	sc.DesignSpecAbs, sc.DesignSpecRel = absDesign, relDesign
	if strings.EqualFold(prefix, "EVAL") {
		// 使用默认的 requirements spec 路径
		reqRaw := strings.TrimSpace(getEnv(prefix+"_REQUIREMENTS_SPEC_PATH", ".spec/REQ-00001.md"))
		absReq, relReq, err := resolvePathUnderRoot(root, reqRaw)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("%s_REQUIREMENTS_SPEC_PATH: %w", prefix, err)
		}
		sc.RequirementsSpecAbs, sc.RequirementsSpecRel = absReq, relReq
	}
	if strings.EqualFold(prefix, "REQUIREMENTS") {
		// 使用默认的 spec 目录
		specDirRaw := strings.TrimSpace(getEnv(prefix+"_SPEC_DIR", ".spec"))
		absSpecDir, relSpecDir, err := resolvePathUnderRoot(root, specDirRaw)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("%s_SPEC_DIR: %w", prefix, err)
		}
		sc.RequirementsSpecDirAbs, sc.RequirementsSpecDirRel = absSpecDir, relSpecDir
	}
	return sc, nil
}

func (c *Config) Validate() error {
	if c.Base.CmdTimeoutSec <= 0 {
		return errors.New("CMD_TIMEOUT_SEC must be > 0")
	}
	if c.Base.BuildMaxRetries <= 0 {
		return errors.New("BUILD_MAX_RETRIES must be > 0")
	}
	for name, sc := range c.Agent {
		if sc.OpenAIBaseURL == "" || sc.OpenAIAPIKey == "" || sc.OpenAIModel == "" {
			return fmt.Errorf("%s agent OPENAI config is required and fully isolated", strings.ToUpper(name))
		}
		if sc.ExecutorMaxIterations <= 0 || sc.PlanExecuteMaxIterations <= 0 {
			return fmt.Errorf("%s agent iterations must be > 0", strings.ToUpper(name))
		}
	}
	return nil
}

// ValidateWithWarnings 验证配置，对于缺失的 OpenAI 配置只打印警告而不返回错误
func (c *Config) ValidateWithWarnings() error {
	if c.Base.CmdTimeoutSec <= 0 {
		return errors.New("CMD_TIMEOUT_SEC must be > 0")
	}
	if c.Base.BuildMaxRetries <= 0 {
		return errors.New("BUILD_MAX_RETRIES must be > 0")
	}

	// 检查 OpenAI 配置，如果缺失则打印警告
	hasValidConfig := false
	for name, sc := range c.Agent {
		if sc.OpenAIAPIKey == "" {
			fmt.Printf("Warning: %s agent OPENAI_API_KEY is not configured. Please configure it in .env file or environment variables.\n", strings.ToUpper(name))
		} else {
			hasValidConfig = true
		}
		if sc.ExecutorMaxIterations <= 0 || sc.PlanExecuteMaxIterations <= 0 {
			return fmt.Errorf("%s agent iterations must be > 0", strings.ToUpper(name))
		}
	}

	if !hasValidConfig {
		fmt.Println("Warning: No agent has valid OpenAI configuration. Please configure at least one agent in .env file.")
		fmt.Println("You can access the system configuration page at http://localhost:8080/#config to set up the configuration.")
	}

	return nil
}

func resolvePathUnderRoot(rootAbs, userPath string) (abs string, rel string, err error) {
	cleanRoot, err := filepath.Abs(rootAbs)
	if err != nil {
		return "", "", err
	}
	var joined string
	if filepath.IsAbs(userPath) {
		joined = filepath.Clean(userPath)
	} else {
		joined = filepath.Join(cleanRoot, filepath.Clean(userPath))
	}
	absPath, err := filepath.Abs(joined)
	if err != nil {
		return "", "", err
	}
	relPath, err := filepath.Rel(cleanRoot, absPath)
	if err != nil {
		return "", "", err
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", errors.New("must be inside WORKSPACE_ROOT")
	}
	return absPath, relPath, nil
}

func getEnv(key, fallback string) string {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	return v
}

func getEnvInt(key string, fallback int) int {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func getEnvBool(key string, fallback bool) bool {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return fallback
	}
	return b
}
