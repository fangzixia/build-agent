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

type Config struct {
	Base  BaseConfig
	Agent map[string]AgentConfig
}

func Load() (*Config, error) {
	_ = godotenv.Load(".env")
	c := &Config{
		Base: BaseConfig{
			WorkspaceRoot:   os.Getenv("WORKSPACE_ROOT"),
			CmdTimeoutSec:   getEnvInt("CMD_TIMEOUT_SEC", 90),
			HTTPAddr:        getEnv("HTTP_ADDR", ":8080"),
			BuildMaxRetries: getEnvInt("BUILD_MAX_RETRIES", 5),
		},
		Agent: make(map[string]AgentConfig, 5),
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
	return c, c.Validate()
}

func loadAgent(prefix, root string) (AgentConfig, error) {
	sc := AgentConfig{
		Name:                     strings.ToLower(prefix),
		OpenAIBaseURL:            os.Getenv(prefix + "_OPENAI_BASE_URL"),
		OpenAIAPIKey:             os.Getenv(prefix + "_OPENAI_API_KEY"),
		OpenAIModel:              os.Getenv(prefix + "_OPENAI_MODEL"),
		ExecutorMaxIterations:    getEnvInt(prefix+"_EXECUTOR_MAX_ITERATIONS", 100),
		PlanExecuteMaxIterations: getEnvInt(prefix+"_PLAN_EXECUTE_MAX_ITERATIONS", 100),
	}
	designFallback := ".spec/design.md"
	if prefix == "ANALYSIS" || prefix == "EVAL" {
		designFallback = getEnv("DESIGN_SPEC_PATH", ".spec/design.md")
	}
	designRaw := strings.TrimSpace(getEnv(prefix+"_DESIGN_SPEC_PATH", designFallback))
	absDesign, relDesign, err := resolvePathUnderRoot(root, designRaw)
	if err != nil {
		return AgentConfig{}, fmt.Errorf("%s_DESIGN_SPEC_PATH: %w", prefix, err)
	}
	sc.DesignSpecAbs, sc.DesignSpecRel = absDesign, relDesign
	if strings.EqualFold(prefix, "EVAL") {
		reqRaw := strings.TrimSpace(getEnv(prefix+"_REQUIREMENTS_SPEC_PATH", getEnv("REQUIREMENTS_SPEC_PATH", ".spec/REQ-00001.md")))
		absReq, relReq, err := resolvePathUnderRoot(root, reqRaw)
		if err != nil {
			return AgentConfig{}, fmt.Errorf("%s_REQUIREMENTS_SPEC_PATH: %w", prefix, err)
		}
		sc.RequirementsSpecAbs, sc.RequirementsSpecRel = absReq, relReq
	}
	if strings.EqualFold(prefix, "REQUIREMENTS") {
		specDirRaw := strings.TrimSpace(getEnv(prefix+"_SPEC_DIR", getEnv("REQUIREMENTS_SPEC_DIR", ".spec")))
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
