package desktop

import (
	"context"
	"os"
	"testing"

	"build-agent/internal/config"
)

func TestNewBridge(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "/test/workspace",
			HTTPAddr:      ":8080",
		},
		Desktop: config.DesktopConfig{
			WindowTitle:  "Build Agent",
			WindowWidth:  1280,
			WindowHeight: 800,
			MinWidth:     800,
			MinHeight:    600,
			EnableTray:   true,
		},
	}

	bridge := NewBridge(cfg)
	if bridge == nil {
		t.Fatal("NewBridge returned nil")
	}

	if bridge.cfg != cfg {
		t.Error("Bridge config not set correctly")
	}
}

func TestBridgeStartup(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "/test/workspace",
		},
		Desktop: config.DesktopConfig{
			WindowTitle: "Build Agent",
		},
	}

	bridge := NewBridge(cfg)
	ctx := context.Background()

	bridge.Startup(ctx)

	if bridge.ctx != ctx {
		t.Error("Bridge context not set correctly in Startup")
	}
}

func TestBridgeShutdown(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "/test/workspace",
		},
		Desktop: config.DesktopConfig{
			WindowTitle: "Build Agent",
		},
	}

	bridge := NewBridge(cfg)
	ctx := context.Background()

	// Initialize bridge
	bridge.Startup(ctx)

	// Call shutdown - should not panic
	bridge.Shutdown(ctx)

	t.Log("Shutdown completed successfully")
}

func TestBridgeGetConfig(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "/test/workspace",
			HTTPAddr:      ":8080",
		},
		Desktop: config.DesktopConfig{
			WindowTitle:  "Build Agent",
			WindowWidth:  1280,
			WindowHeight: 800,
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	result, err := bridge.GetConfig()
	if err != nil {
		t.Fatalf("GetConfig returned error: %v", err)
	}

	if result["workspaceRoot"] != "/test/workspace" {
		t.Errorf("Expected workspaceRoot to be '/test/workspace', got %v", result["workspaceRoot"])
	}

	if result["httpAddr"] != ":8080" {
		t.Errorf("Expected httpAddr to be ':8080', got %v", result["httpAddr"])
	}
}

func TestBridgeRunTask(t *testing.T) {
	// Test with empty config to trigger error path
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "../../",
			HTTPAddr:      ":8080",
		},
		Agent: map[string]config.AgentConfig{},
	}

	bridge := NewBridge(cfg)
	ctx := context.Background()
	bridge.Startup(ctx)

	// Test that the method handles missing agent gracefully
	_, err := bridge.RunTask("nonexistent", "test task")

	if err == nil {
		t.Error("Expected error for nonexistent agent, got nil")
	}
	t.Logf("RunTask correctly returned error: %v", err)
}

func TestBridgeRunTaskWithProgress(t *testing.T) {
	// Test with empty config to trigger error path
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "../../",
			HTTPAddr:      ":8080",
		},
		Agent: map[string]config.AgentConfig{},
	}

	bridge := NewBridge(cfg)
	ctx := context.Background()
	bridge.Startup(ctx)

	// Test that the method handles missing agent gracefully
	_, err := bridge.RunTaskWithProgress("nonexistent", "test task")

	if err == nil {
		t.Error("Expected error for nonexistent agent, got nil")
	}
	t.Logf("RunTaskWithProgress correctly returned error: %v", err)
}

func TestBridgeRunTaskInvalidAgent(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "../../",
			HTTPAddr:      ":8080",
		},
		Agent: map[string]config.AgentConfig{},
	}

	bridge := NewBridge(cfg)
	ctx := context.Background()
	bridge.Startup(ctx)

	// Test with invalid agent name
	_, err := bridge.RunTask("invalid_agent", "test task")

	if err == nil {
		t.Error("Expected error for invalid agent name, got nil")
	}
	t.Logf("Correctly returned error for invalid agent: %v", err)
}

func TestBridgeRunTaskWithProgressInvalidAgent(t *testing.T) {
	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: "../../",
			HTTPAddr:      ":8080",
		},
		Agent: map[string]config.AgentConfig{},
	}

	bridge := NewBridge(cfg)
	ctx := context.Background()
	bridge.Startup(ctx)

	// Test with invalid agent name
	_, err := bridge.RunTaskWithProgress("invalid_agent", "test task")

	if err == nil {
		t.Error("Expected error for invalid agent name, got nil")
	}
	t.Logf("Correctly returned error for invalid agent: %v", err)
}

func TestBridgeReadFile(t *testing.T) {
	// Create a temporary directory for testing
	tmpDir := t.TempDir()

	// Create a test file
	testContent := "Hello, World!"
	testFile := "test.txt"
	testPath := tmpDir + "/" + testFile
	if err := os.WriteFile(testPath, []byte(testContent), 0644); err != nil {
		t.Fatalf("Failed to create test file: %v", err)
	}

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test reading the file
	content, err := bridge.ReadFile(testFile)
	if err != nil {
		t.Fatalf("ReadFile returned error: %v", err)
	}

	if content != testContent {
		t.Errorf("Expected content '%s', got '%s'", testContent, content)
	}
}

func TestBridgeReadFileOutsideWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Try to read a file outside the workspace
	_, err := bridge.ReadFile("../../../etc/passwd")
	if err == nil {
		t.Error("Expected error when reading file outside workspace, got nil")
	}
	t.Logf("Correctly rejected path outside workspace: %v", err)
}

func TestBridgeSaveFile(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test saving a file
	testContent := "Test content for save"
	testFile := "subdir/newfile.txt"

	err := bridge.SaveFile(testFile, testContent)
	if err != nil {
		t.Fatalf("SaveFile returned error: %v", err)
	}

	// Verify the file was created
	savedPath := tmpDir + "/" + testFile
	content, err := os.ReadFile(savedPath)
	if err != nil {
		t.Fatalf("Failed to read saved file: %v", err)
	}

	if string(content) != testContent {
		t.Errorf("Expected saved content '%s', got '%s'", testContent, string(content))
	}
}

func TestBridgeSaveFileOutsideWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Try to save a file outside the workspace
	err := bridge.SaveFile("../../../tmp/malicious.txt", "bad content")
	if err == nil {
		t.Error("Expected error when saving file outside workspace, got nil")
	}
	t.Logf("Correctly rejected path outside workspace: %v", err)
}

func TestBridgeListFiles(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test files and directories
	os.WriteFile(tmpDir+"/file1.txt", []byte("content1"), 0644)
	os.WriteFile(tmpDir+"/file2.txt", []byte("content2"), 0644)
	os.Mkdir(tmpDir+"/subdir", 0755)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test listing files
	files, err := bridge.ListFiles(".")
	if err != nil {
		t.Fatalf("ListFiles returned error: %v", err)
	}

	if len(files) != 3 {
		t.Errorf("Expected 3 files, got %d", len(files))
	}

	// Check that we have the expected files
	fileNames := make(map[string]bool)
	for _, f := range files {
		fileNames[f.Name] = true
		t.Logf("Found file: %s (IsDir: %v, Size: %d)", f.Name, f.IsDir, f.Size)
	}

	if !fileNames["file1.txt"] || !fileNames["file2.txt"] || !fileNames["subdir"] {
		t.Error("Missing expected files in listing")
	}
}

func TestBridgeListFilesOutsideWorkspace(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Try to list files outside the workspace
	_, err := bridge.ListFiles("../../..")
	if err == nil {
		t.Error("Expected error when listing files outside workspace, got nil")
	}
	t.Logf("Correctly rejected path outside workspace: %v", err)
}

func TestBridgeReadFileNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Try to read a non-existent file
	_, err := bridge.ReadFile("nonexistent.txt")
	if err == nil {
		t.Error("Expected error when reading non-existent file, got nil")
	}
	t.Logf("Correctly returned error for non-existent file: %v", err)
}

func TestBridgeListFilesNonExistentDir(t *testing.T) {
	tmpDir := t.TempDir()

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Try to list a non-existent directory
	_, err := bridge.ListFiles("nonexistent_dir")
	if err == nil {
		t.Error("Expected error when listing non-existent directory, got nil")
	}
	t.Logf("Correctly returned error for non-existent directory: %v", err)
}

func TestBridgeGetEnvConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test .env file
	envContent := `WORKSPACE_ROOT=/test/workspace
CMD_TIMEOUT_SEC=90
HTTP_ADDR=:8080

# code scenario (OPENAI fully isolated)
CODE_OPENAI_BASE_URL=https://api.openai.com/v1
CODE_OPENAI_API_KEY=test_key
CODE_OPENAI_MODEL=gpt-4o-mini
`
	envPath := tmpDir + "/.env"
	if err := os.WriteFile(envPath, []byte(envContent), 0644); err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test reading env config
	envConfig, err := bridge.GetEnvConfig()
	if err != nil {
		t.Fatalf("GetEnvConfig returned error: %v", err)
	}

	if envConfig == nil {
		t.Fatal("GetEnvConfig returned nil config")
	}

	if len(envConfig.Sections) == 0 {
		t.Error("Expected non-empty sections in env config")
	}
}

func TestBridgeGetEnvConfigNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory (no .env file)
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test reading env config when file doesn't exist
	envConfig, err := bridge.GetEnvConfig()
	if err != nil {
		t.Fatalf("GetEnvConfig returned error: %v", err)
	}

	// Should return empty config
	if envConfig == nil {
		t.Fatal("GetEnvConfig returned nil config")
	}

	if envConfig.Sections == nil {
		t.Error("Expected initialized sections map")
	}
}

func TestBridgeGetEnvConfigWithExample(t *testing.T) {
	tmpDir := t.TempDir()

	// Create only .env.example file
	exampleContent := `WORKSPACE_ROOT=/example/workspace
CMD_TIMEOUT_SEC=90
HTTP_ADDR=:8080
`
	examplePath := tmpDir + "/.env.example"
	if err := os.WriteFile(examplePath, []byte(exampleContent), 0644); err != nil {
		t.Fatalf("Failed to create test .env.example file: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test reading env config (should fall back to .env.example)
	envConfig, err := bridge.GetEnvConfig()
	if err != nil {
		t.Fatalf("GetEnvConfig returned error: %v", err)
	}

	if envConfig == nil {
		t.Fatal("GetEnvConfig returned nil config")
	}

	if len(envConfig.Sections) == 0 {
		t.Error("Expected non-empty sections from .env.example")
	}
}

func TestBridgeSaveEnvConfig(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Create a test env config
	envConfig := &config.EnvConfig{
		Sections: map[string][]config.EnvEntry{
			"Base": {
				{Key: "WORKSPACE_ROOT", Value: tmpDir, Section: "Base"},
				{Key: "CMD_TIMEOUT_SEC", Value: "90", Section: "Base"},
				{Key: "HTTP_ADDR", Value: ":8080", Section: "Base"},
			},
		},
	}

	// Test saving env config
	err := bridge.SaveEnvConfig(envConfig)
	if err != nil {
		t.Fatalf("SaveEnvConfig returned error: %v", err)
	}

	// Verify the file was created
	envPath := tmpDir + "/.env"
	content, err := os.ReadFile(envPath)
	if err != nil {
		t.Fatalf("Failed to read saved .env file: %v", err)
	}

	contentStr := string(content)
	if !contains(contentStr, "WORKSPACE_ROOT") {
		t.Error("Expected WORKSPACE_ROOT in saved .env file")
	}
	if !contains(contentStr, "CMD_TIMEOUT_SEC") {
		t.Error("Expected CMD_TIMEOUT_SEC in saved .env file")
	}
}

func TestBridgeGetEnvConfigExample(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test .env.example file
	exampleContent := `WORKSPACE_ROOT=/example/workspace
CMD_TIMEOUT_SEC=90
HTTP_ADDR=:8080

# code scenario (OPENAI fully isolated)
CODE_OPENAI_BASE_URL=https://api.openai.com/v1
CODE_OPENAI_API_KEY=your_key_here
CODE_OPENAI_MODEL=gpt-4o-mini
`
	examplePath := tmpDir + "/.env.example"
	if err := os.WriteFile(examplePath, []byte(exampleContent), 0644); err != nil {
		t.Fatalf("Failed to create test .env.example file: %v", err)
	}

	// Change to temp directory
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test reading env config example
	envConfig, err := bridge.GetEnvConfigExample()
	if err != nil {
		t.Fatalf("GetEnvConfigExample returned error: %v", err)
	}

	if envConfig == nil {
		t.Fatal("GetEnvConfigExample returned nil config")
	}

	if len(envConfig.Sections) == 0 {
		t.Error("Expected non-empty sections in env config example")
	}
}

func TestBridgeGetEnvConfigExampleNonExistent(t *testing.T) {
	tmpDir := t.TempDir()

	// Change to temp directory (no .env.example file)
	oldWd, _ := os.Getwd()
	defer os.Chdir(oldWd)
	os.Chdir(tmpDir)

	cfg := &config.Config{
		Base: config.BaseConfig{
			WorkspaceRoot: tmpDir,
			HTTPAddr:      ":8080",
		},
	}

	bridge := NewBridge(cfg)
	bridge.Startup(context.Background())

	// Test reading env config example when file doesn't exist
	_, err := bridge.GetEnvConfigExample()
	if err == nil {
		t.Error("Expected error when .env.example doesn't exist, got nil")
	}
	t.Logf("Correctly returned error for non-existent .env.example: %v", err)
}

// Helper function for string contains check
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && indexOf(s, substr) >= 0))
}

func indexOf(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
