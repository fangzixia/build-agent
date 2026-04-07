package desktop

import (
	"context"
	"os"
	"testing"

	"build-agent/internal/config"
)

func newTestBridge(workspaceRoot string) *Bridge {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot:   workspaceRoot,
			CmdTimeoutSec:   90,
			BuildMaxRetries: 5,
		},
		Agent: map[string]config.AgentConfig{},
	}
	b := NewBridge(cfg)
	b.Startup(context.Background())
	return b
}

func TestNewBridge(t *testing.T) {
	b := newTestBridge(t.TempDir())
	if b == nil {
		t.Fatal("NewBridge returned nil")
	}
}

func TestBridgeStartupShutdown(t *testing.T) {
	b := newTestBridge(t.TempDir())
	ctx := context.Background()
	if b.ctx != ctx {
		t.Error("context not set correctly")
	}
	b.Shutdown(ctx) // should not panic
}

func TestBridgeRunTaskInvalidAgent(t *testing.T) {
	b := newTestBridge(t.TempDir())
	_, err := b.RunTask("nonexistent", "test task")
	if err == nil {
		t.Error("expected error for nonexistent agent")
	}
}

func TestBridgeReadFile(t *testing.T) {
	tmpDir := t.TempDir()
	content := "Hello, World!"
	if err := os.WriteFile(tmpDir+"/test.txt", []byte(content), 0644); err != nil {
		t.Fatal(err)
	}

	b := newTestBridge(tmpDir)
	got, err := b.ReadFile("test.txt")
	if err != nil {
		t.Fatalf("ReadFile error: %v", err)
	}
	if got != content {
		t.Errorf("content = %q, want %q", got, content)
	}
}

func TestBridgeReadFileOutsideWorkspace(t *testing.T) {
	b := newTestBridge(t.TempDir())
	_, err := b.ReadFile("../../../etc/passwd")
	if err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestBridgeSaveFile(t *testing.T) {
	tmpDir := t.TempDir()
	b := newTestBridge(tmpDir)

	if err := b.SaveFile("subdir/new.txt", "content"); err != nil {
		t.Fatalf("SaveFile error: %v", err)
	}
	data, err := os.ReadFile(tmpDir + "/subdir/new.txt")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "content" {
		t.Errorf("saved content = %q, want %q", string(data), "content")
	}
}

func TestBridgeSaveFileOutsideWorkspace(t *testing.T) {
	b := newTestBridge(t.TempDir())
	if err := b.SaveFile("../../../tmp/bad.txt", "bad"); err == nil {
		t.Error("expected error for path outside workspace")
	}
}

func TestBridgeListFiles(t *testing.T) {
	tmpDir := t.TempDir()
	os.WriteFile(tmpDir+"/a.txt", []byte("a"), 0644)
	os.WriteFile(tmpDir+"/b.txt", []byte("b"), 0644)
	os.Mkdir(tmpDir+"/sub", 0755)

	b := newTestBridge(tmpDir)
	files, err := b.ListFiles(".")
	if err != nil {
		t.Fatalf("ListFiles error: %v", err)
	}
	if len(files) != 3 {
		t.Errorf("len(files) = %d, want 3", len(files))
	}
}

func TestBridgeGetEnvConfigDefault(t *testing.T) {
	b := newTestBridge(t.TempDir())
	s, err := b.GetSettings()
	if err != nil {
		t.Fatalf("GetSettings error: %v", err)
	}
	if s == nil {
		t.Error("expected non-nil settings")
	}
}
