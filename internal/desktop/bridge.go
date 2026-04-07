package desktop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"

	"build-agent/internal/applog"
	"build-agent/internal/config"
	"build-agent/internal/core"
	"build-agent/internal/toolkit"
)

// Bridge 提供前端可调用的 Go 方法
type Bridge struct {
	ctx context.Context
	cfg *config.Config
}

func NewBridge(cfg *config.Config) *Bridge {
	return &Bridge{cfg: cfg}
}

func (b *Bridge) Startup(ctx context.Context) {
	b.ctx = ctx
}

func (b *Bridge) Shutdown(_ context.Context) {}

// RunTask 执行任务（非流式）
func (b *Bridge) RunTask(agentName, task string) (*core.RunResult, error) {
	applog.Bridge("RunTask", map[string]any{"agent": agentName, "task": task})
	svc, err := core.NewService(b.ctx, b.cfg, agentName)
	if err != nil {
		applog.BridgeError("RunTask", err, map[string]any{"agent": agentName})
		return nil, err
	}
	result, err := svc.RunTask(b.ctx, task)
	if err != nil {
		applog.BridgeError("RunTask", err, map[string]any{"agent": agentName})
	} else {
		applog.Bridge("RunTask.done", map[string]any{"agent": agentName, "has_error": result.HasError})
	}
	return result, err
}

// RunTaskWithProgress 执行任务并流式返回进度
func (b *Bridge) RunTaskWithProgress(agentName, task string) (*core.RunResult, error) {
	applog.Bridge("RunTaskWithProgress", map[string]any{"agent": agentName, "task": task})
	svc, err := core.NewService(b.ctx, b.cfg, agentName)
	if err != nil {
		applog.BridgeError("RunTaskWithProgress", err, map[string]any{"agent": agentName})
		return nil, err
	}
	result, err := svc.RunTaskWithProgress(b.ctx, task, func(log core.EventLog) {
		runtime.EventsEmit(b.ctx, "task:progress", log)
	})
	if err != nil {
		applog.BridgeError("RunTaskWithProgress", err, map[string]any{"agent": agentName})
	} else {
		applog.Bridge("RunTaskWithProgress.done", map[string]any{"agent": agentName, "has_error": result.HasError})
	}
	return result, err
}

// GetSettings 读取用户配置
func (b *Bridge) GetSettings() (*config.Settings, error) {
	applog.Bridge("GetSettings", nil)
	s, err := config.LoadSettings()
	if err != nil {
		applog.BridgeError("GetSettings", err, nil)
	}
	return s, err
}

// SaveSettings 保存用户配置并热重载
func (b *Bridge) SaveSettings(s *config.Settings) error {
	applog.Bridge("SaveSettings", map[string]any{"model": s.Model.Model})
	if err := config.SaveSettings(s); err != nil {
		applog.BridgeError("SaveSettings", err, nil)
		return fmt.Errorf("save settings: %w", err)
	}
	newCfg, err := config.Load()
	if err != nil {
		applog.BridgeError("SaveSettings.reload", err, nil)
		return fmt.Errorf("reload config: %w", err)
	}
	b.cfg = newCfg
	return nil
}

// GetWorkspace 返回当前工作区和最近列表
func (b *Bridge) GetWorkspace() (map[string]interface{}, error) {
	applog.Bridge("GetWorkspace", map[string]any{"current": b.cfg.Base.WorkspaceRoot})
	return map[string]interface{}{
		"current": b.cfg.Base.WorkspaceRoot,
		"recent":  loadWorkspaceHistory(),
	}, nil
}

// SetWorkspace 切换工作区
func (b *Bridge) SetWorkspace(path string) error {
	applog.Bridge("SetWorkspace", map[string]any{"path": path})
	info, err := os.Stat(path)
	if err != nil || !info.IsDir() {
		err := fmt.Errorf("path does not exist or is not a directory")
		applog.BridgeError("SetWorkspace", err, map[string]any{"path": path})
		return err
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		applog.BridgeError("SetWorkspace", err, map[string]any{"path": path})
		return fmt.Errorf("invalid path")
	}
	if err := os.Setenv("WORKSPACE_ROOT", abs); err != nil {
		applog.BridgeError("SetWorkspace", err, nil)
		return err
	}
	newCfg, err := config.Load()
	if err != nil {
		applog.BridgeError("SetWorkspace.reload", err, nil)
		return err
	}
	b.cfg = newCfg
	addToWorkspaceHistory(abs)
	return nil
}

// OpenFolderDialog 打开系统文件夹选择对话框
func (b *Bridge) OpenFolderDialog() (string, error) {
	return runtime.OpenDirectoryDialog(b.ctx, runtime.OpenDialogOptions{
		Title: "选择工作区文件夹",
	})
}

// ReadFile 读取工作区内的文件
func (b *Bridge) ReadFile(path string) (string, error) {
	safePath, err := toolkit.ResolveSafePath(b.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return "", fmt.Errorf("invalid path: %w", err)
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return "", fmt.Errorf("read file: %w", err)
	}
	return string(content), nil
}

// SaveFile 保存文件到工作区
func (b *Bridge) SaveFile(path, content string) error {
	safePath, err := toolkit.ResolveSafePath(b.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return fmt.Errorf("invalid path: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(safePath), 0755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	return os.WriteFile(safePath, []byte(content), 0644)
}

// FileInfo 文件信息
type FileInfo struct {
	Name  string `json:"name"`
	IsDir bool   `json:"isDir"`
	Size  int64  `json:"size"`
}

// ListFiles 列出目录内容
func (b *Bridge) ListFiles(path string) ([]FileInfo, error) {
	safePath, err := toolkit.ResolveSafePath(b.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return nil, fmt.Errorf("invalid path: %w", err)
	}
	entries, err := os.ReadDir(safePath)
	if err != nil {
		return nil, fmt.Errorf("read directory: %w", err)
	}
	result := make([]FileInfo, 0, len(entries))
	for _, entry := range entries {
		info, err := entry.Info()
		if err != nil {
			continue
		}
		result = append(result, FileInfo{Name: entry.Name(), IsDir: entry.IsDir(), Size: info.Size()})
	}
	return result, nil
}

// --- 工作区历史 ---

type workspaceEntry struct {
	Path     string    `json:"path"`
	LastUsed time.Time `json:"lastUsed"`
}

func workspaceHistoryPath() string {
	base, err := os.UserConfigDir()
	if err != nil {
		base, _ = os.UserHomeDir()
	}
	dir := filepath.Join(base, "build-agent")
	_ = os.MkdirAll(dir, 0755)
	return filepath.Join(dir, "workspaces.json")
}

func loadWorkspaceHistory() []workspaceEntry {
	p := workspaceHistoryPath()
	if p == "" {
		return nil
	}
	data, err := os.ReadFile(p)
	if err != nil {
		return nil
	}
	var result struct {
		Recent []workspaceEntry `json:"recent"`
	}
	if err := json.Unmarshal(data, &result); err != nil {
		return nil
	}
	return result.Recent
}

func addToWorkspaceHistory(path string) {
	existing := loadWorkspaceHistory()
	filtered := existing[:0]
	for _, e := range existing {
		if e.Path != path {
			filtered = append(filtered, e)
		}
	}
	all := append([]workspaceEntry{{Path: path, LastUsed: time.Now()}}, filtered...)
	if len(all) > 10 {
		all = all[:10]
	}
	p := workspaceHistoryPath()
	if p == "" {
		return
	}
	data, _ := json.MarshalIndent(map[string]interface{}{"recent": all}, "", "  ")
	_ = os.WriteFile(p, data, 0644)
}

// RequirementInfo 需求文件信息
type RequirementInfo struct {
	ID       string `json:"id"`
	Path     string `json:"path"`
	Title    string `json:"title"`
	FullPath string `json:"fullPath"`
}

// EvaluationInfo 评测文件信息
type EvaluationInfo struct {
	ID            string `json:"id"`
	RequirementID string `json:"requirementId"`
	Path          string `json:"path"`
	FullPath      string `json:"fullPath"`
}

// GetRequirements 列出工作区 .spec 下的需求文件
func (b *Bridge) GetRequirements() ([]RequirementInfo, error) {
	specDir := filepath.Join(b.cfg.Base.WorkspaceRoot, ".spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []RequirementInfo{}, nil
		}
		return nil, err
	}
	var result []RequirementInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if !strings.HasPrefix(e.Name(), "REQ-") {
			continue
		}
		id := strings.TrimSuffix(e.Name(), ".md")
		relPath := filepath.ToSlash(filepath.Join(".spec", e.Name()))
		result = append(result, RequirementInfo{
			ID:       id,
			Path:     relPath,
			Title:    id,
			FullPath: filepath.Join(specDir, e.Name()),
		})
	}
	return result, nil
}

// GetEvaluations 列出工作区 .spec 下的评测文件
func (b *Bridge) GetEvaluations() ([]EvaluationInfo, error) {
	specDir := filepath.Join(b.cfg.Base.WorkspaceRoot, ".spec")
	entries, err := os.ReadDir(specDir)
	if err != nil {
		if os.IsNotExist(err) {
			return []EvaluationInfo{}, nil
		}
		return nil, err
	}
	var result []EvaluationInfo
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".md") {
			continue
		}
		if !strings.HasPrefix(e.Name(), "EVAL-") {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ".md") // e.g. EVAL-REQ-00001-01
		parts := strings.SplitN(name, "-", 4)       // ["EVAL", "REQ", "00001", "01"]
		reqID := ""
		if len(parts) >= 3 {
			reqID = "REQ-" + parts[2]
		}
		relPath := filepath.ToSlash(filepath.Join(".spec", e.Name()))
		result = append(result, EvaluationInfo{
			ID:            name,
			RequirementID: reqID,
			Path:          relPath,
			FullPath:      filepath.Join(specDir, e.Name()),
		})
	}
	return result, nil
}
