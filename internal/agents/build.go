package agents

import (
	"fmt"

	"build-agent/internal/config"
)

func Build(name string, cfg *config.Config) (Agent, error) {
	sc, ok := cfg.Agent[name]
	if !ok {
		return nil, fmt.Errorf("unknown agent: %s", name)
	}
	switch name {
	case "code":
		return buildCodeAgent(cfg.Base.WorkspaceRoot), nil
	case "analysis":
		return buildAnalysisAgent(cfg.Base.WorkspaceRoot, sc), nil
	case "eval":
		return buildEvalAgent(cfg.Base.WorkspaceRoot, sc, cfg.Base.CmdTimeoutSec), nil
	case "requirements":
		return buildRequirementsAgent(cfg.Base.WorkspaceRoot, sc), nil
	case "build":
		return buildBuildAgent(cfg.Base.WorkspaceRoot, sc), nil
	default:
		return nil, fmt.Errorf("unknown agent: %s", name)
	}
}
