package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type BaseConfig struct {
	WorkspaceRoot   string
	CmdTimeoutSec   int
	BuildMaxRetries int
}

type ModelConfig struct {
	BaseURL                string
	APIKey                 string
	Model                  string
	MaxContextTokens       int
	SmartCompressThreshold int
}

type AgentConfig struct {
	Name                     string
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
	Model ModelConfig
	Agent map[string]AgentConfig
}

func Load() (*Config, error) {
	s, err := LoadSettings()
	if err != nil {
		return nil, fmt.Errorf("load settings: %w", err)
	}

	workspaceRoot := os.Getenv("WORKSPACE_ROOT")
	if workspaceRoot == "" {
		// 优先使用最近打开的工作区
		if last := LastWorkspace(); last != "" {
			workspaceRoot = last
		} else {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("get home directory: %w", err)
			}
			workspaceRoot = home
		}
	}
	absRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve WORKSPACE_ROOT: %w", err)
	}

	c := &Config{
		Base: BaseConfig{
			WorkspaceRoot:   absRoot,
			CmdTimeoutSec:   90,
			BuildMaxRetries: 5,
		},
		Model: ModelConfig{
			BaseURL:                s.Model.BaseURL,
			APIKey:                 s.Model.APIKey,
			Model:                  s.Model.Model,
			MaxContextTokens:       s.Model.MaxContextTokens,
			SmartCompressThreshold: s.Model.SmartCompressThreshold,
		},
		Agent: make(map[string]AgentConfig, 5),
	}

	for _, name := range []string{"CODE", "ANALYSIS", "EVAL", "REQUIREMENTS", "BUILD", "CHAT"} {
		lower := strings.ToLower(name)
		as := s.Agents[lower]
		ac, err := buildAgentConfig(lower, absRoot, as)
		if err != nil {
			return nil, err
		}
		c.Agent[lower] = ac
	}

	return c, c.validate()
}

func buildAgentConfig(name, root string, as AgentSettings) (AgentConfig, error) {
	ac := AgentConfig{
		Name:                     name,
		ExecutorMaxIterations:    as.ExecutorMaxIterations,
		PlanExecuteMaxIterations: as.PlanExecuteMaxIterations,
	}
	if ac.ExecutorMaxIterations <= 0 {
		ac.ExecutorMaxIterations = 100
	}
	if ac.PlanExecuteMaxIterations <= 0 {
		ac.PlanExecuteMaxIterations = 10
	}

	absDesign, relDesign, err := resolvePathUnderRoot(root, ".spec/design.md")
	if err != nil {
		return AgentConfig{}, fmt.Errorf("%s design spec path: %w", name, err)
	}
	ac.DesignSpecAbs, ac.DesignSpecRel = absDesign, relDesign

	if name == "eval" {
		absReq, relReq, err := resolvePathUnderRoot(root, ".spec/REQ-00001.md")
		if err != nil {
			return AgentConfig{}, fmt.Errorf("eval requirements spec path: %w", err)
		}
		ac.RequirementsSpecAbs, ac.RequirementsSpecRel = absReq, relReq
	}

	if name == "requirements" {
		absDir, relDir, err := resolvePathUnderRoot(root, ".spec")
		if err != nil {
			return AgentConfig{}, fmt.Errorf("requirements spec dir: %w", err)
		}
		ac.RequirementsSpecDirAbs, ac.RequirementsSpecDirRel = absDir, relDir
	}

	return ac, nil
}

func (c *Config) validate() error {
	if c.Base.CmdTimeoutSec <= 0 {
		return errors.New("CmdTimeoutSec must be > 0")
	}
	if c.Base.BuildMaxRetries <= 0 {
		return errors.New("BuildMaxRetries must be > 0")
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
