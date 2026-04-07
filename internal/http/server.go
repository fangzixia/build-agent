package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"build-agent/internal/config"
	"build-agent/internal/core"
)

type Server struct {
	cfg          *config.Config
	defaultAgent string
}

func New(cfg *config.Config, agentName string) *Server {
	if agentName == "" {
		agentName = "code"
	}
	return &Server{cfg: cfg, defaultAgent: agentName}
}

type runRequest struct {
	Task     string `json:"task"`
	FilePath string `json:"filePath,omitempty"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()

	// 静态文件服务
	mux.Handle("/static/", http.StripPrefix("/static/", http.FileServer(http.Dir("frontend/static"))))

	// 首页
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			http.ServeFile(w, r, "frontend/index.html")
			return
		}
		http.NotFound(w, r)
	})

	// 健康检查
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})

	// API 端点
	mux.HandleFunc("/v1/analysis/run", s.handleRunForAgent("analysis"))
	mux.HandleFunc("/v1/analysis/chat", s.handleRunForAgent("analysis"))

	mux.HandleFunc("/v1/requirements/run", s.handleRunForAgent("requirements"))
	mux.HandleFunc("/v1/requirements/chat", s.handleRunForAgent("requirements"))

	mux.HandleFunc("/v1/req/run", s.handleRunForAgent("requirements"))
	mux.HandleFunc("/v1/req/chat", s.handleRunForAgent("requirements"))

	mux.HandleFunc("/v1/code/run", s.handleRunForAgent("code"))
	mux.HandleFunc("/v1/code/chat", s.handleRunForAgent("code"))

	mux.HandleFunc("/v1/eval/run", s.handleRunForAgent("eval"))
	mux.HandleFunc("/v1/eval/chat", s.handleRunForAgent("eval"))

	mux.HandleFunc("/v1/build/run", s.handleRunForAgent("build"))
	mux.HandleFunc("/v1/build/chat", s.handleRunForAgent("build"))

	// 文件管理 API
	mux.HandleFunc("/v1/files/list", s.handleListFiles())
	mux.HandleFunc("/v1/files/read", s.handleReadFile())
	mux.HandleFunc("/v1/files/save", s.handleSaveFile())

	// 需求和验收数据 API
	mux.HandleFunc("/v1/requirements", s.handleGetRequirements())
	mux.HandleFunc("/v1/evaluations", s.handleGetEvaluations())
	mux.HandleFunc("/v1/config", s.handleGetConfig())

	// ENV 配置管理 API
	mux.HandleFunc("/v1/config/env", s.handleEnvConfig())
	mux.HandleFunc("/v1/config/env/example", s.handleEnvConfigExample())

	// Backward compatible endpoint: defaults to current server agent.
	mux.HandleFunc("/v1/run", s.handleRunForAgent(s.defaultAgent))
	mux.HandleFunc("/v1/chat", s.handleRunForAgent(s.defaultAgent))

	return mux
}

func (s *Server) ListenAndServe(ctx context.Context, addr string) error {
	srv := &http.Server{Addr: addr, Handler: s.Handler()}
	errCh := make(chan error, 1)
	go func() { errCh <- srv.ListenAndServe() }()
	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return srv.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func (s *Server) handleRunForAgent(agentName string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}
		var req runRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}

		task, err := getTask(agentName, req)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		svc, err := core.NewService(r.Context(), s.cfg, agentName)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		// 检查是否请求流式输出
		if r.Header.Get("Accept") == "text/event-stream" {
			s.handleStreamingExecution(w, r, svc, task)
			return
		}

		// 非流式执行
		result, err := svc.RunTask(r.Context(), task)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func getTask(agentName string, req runRequest) (string, error) {
	task := req.Task
	filePath := req.FilePath

	switch agentName {
	case "analysis":
		{
			if task == "" {
				return "分析项目结构，生成项目信息文档", nil
			}
			return task, nil
		}
	case "requirements":
		if task == "" {
			return "", errors.New("请描述你的需求")
		} else if filePath != "" {
			return fmt.Sprintf("根据用户需求:%s,修改需求文档%s", task, filePath), nil
		}
		return task, nil
	case "code":
		if filePath == "" && task == "" {
			return "", errors.New("请描述你要编码的任务")
		} else if filePath == "" {
			return task, nil
		}
		return fmt.Sprintf("请完成需求文档%s的编码需求", filePath), nil

	case "eval":
		if filePath == "" && task == "" {
			return "", errors.New("请描述你要验收的任务")
		} else if filePath == "" {
			return task, nil
		}
		return fmt.Sprintf("请完成需求文档%s的验收任务", filePath), nil
	case "build":
		if filePath == "" && task == "" {
			return "", errors.New("请描述你要构建的任务")
		} else if filePath == "" {
			return task, nil
		} else if task == "" {
			return fmt.Sprintf("请完成需求文档%s的构建任务", filePath), nil
		}
		return fmt.Sprintf("请根据用户需求:%s,完成需求文档%s的构建任务", task, filePath), nil
	}
	return "", errors.New(fmt.Sprintf("任务类型错误:%s", agentName))
}

func (s *Server) handleStreamingExecution(w http.ResponseWriter, r *http.Request, svc *core.Service, task string) {
	// 设置 SSE 响应头
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		writeJSON(w, http.StatusInternalServerError, map[string]string{"error": "streaming not supported"})
		return
	}

	// 发送开始事件
	sendSSE(w, flusher, "start", map[string]interface{}{
		"message": "开始执行任务",
		"task":    task,
	})

	// 执行任务并流式输出日志
	result, err := svc.RunTaskWithProgress(r.Context(), task, func(log core.EventLog) {
		sendSSE(w, flusher, "log", map[string]interface{}{
			"agent_name": log.AgentName,
			"role":       log.Role,
			"tool_name":  log.ToolName,
			"output":     log.Output,
			"error":      log.Error,
		})
	})

	if err != nil {
		sendSSE(w, flusher, "error", map[string]interface{}{
			"error": err.Error(),
		})
		return
	}

	// 发送完成事件
	sendSSE(w, flusher, "done", result)
}

func sendSSE(w http.ResponseWriter, flusher http.Flusher, event string, data interface{}) {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return
	}
	fmt.Fprintf(w, "event: %s\ndata: %s\n\n", event, jsonData)
	flusher.Flush()
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *Server) handleListFiles() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			path = ".spec"
		}

		fullPath := filepath.Join(s.cfg.Base.WorkspaceRoot, path)
		entries, err := os.ReadDir(fullPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		files := make([]map[string]interface{}, 0)
		for _, entry := range entries {
			info, err := entry.Info()
			if err != nil {
				continue
			}

			files = append(files, map[string]interface{}{
				"name":    entry.Name(),
				"isDir":   entry.IsDir(),
				"size":    info.Size(),
				"modTime": info.ModTime(),
			})
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"path":  path,
			"files": files,
		})
	}
}

func (s *Server) handleReadFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		path := r.URL.Query().Get("path")
		if path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}

		fullPath := filepath.Join(s.cfg.Base.WorkspaceRoot, path)
		content, err := os.ReadFile(fullPath)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"path":    path,
			"content": string(content),
		})
	}
}

func (s *Server) handleSaveFile() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		var req struct {
			Path    string `json:"path"`
			Content string `json:"content"`
		}

		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid json body"})
			return
		}

		if req.Path == "" {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": "path is required"})
			return
		}

		fullPath := filepath.Join(s.cfg.Base.WorkspaceRoot, req.Path)

		// 确保目录存在
		dir := filepath.Dir(fullPath)
		if err := os.MkdirAll(dir, 0755); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		// 写入文件
		if err := os.WriteFile(fullPath, []byte(req.Content), 0644); err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]interface{}{
			"success": true,
			"path":    req.Path,
		})
	}
}

func (s *Server) handleGetConfig() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		// 返回前端需要的配置信息
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"analysisDesignSpecPath":   s.cfg.Agent["analysis"].DesignSpecRel,
			"requirementsSpecDir":      s.cfg.Agent["requirements"].RequirementsSpecDirRel,
			"evalRequirementsSpecPath": s.cfg.Agent["eval"].RequirementsSpecRel,
			"evalPassScoreThreshold":   100,
		})
	}
}

func (s *Server) handleEnvConfig() http.HandlerFunc {
	handler := NewEnvConfigHandler(s.cfg.Base.WorkspaceRoot)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleGetConfig(w, r)
		} else if r.Method == http.MethodPost {
			handler.HandleSaveConfig(w, r)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}

func (s *Server) handleEnvConfigExample() http.HandlerFunc {
	handler := NewEnvConfigHandler(s.cfg.Base.WorkspaceRoot)
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			handler.HandleGetExampleConfig(w, r)
		} else {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
		}
	}
}
