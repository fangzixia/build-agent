package config

import (
	"os"
	"testing"
)

func TestConfig_DefaultValues(t *testing.T) {
	os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Base.CmdTimeoutSec != 90 {
		t.Errorf("CmdTimeoutSec = %v, want 90", cfg.Base.CmdTimeoutSec)
	}
	if cfg.Base.BuildMaxRetries != 5 {
		t.Errorf("BuildMaxRetries = %v, want 5", cfg.Base.BuildMaxRetries)
	}
}
