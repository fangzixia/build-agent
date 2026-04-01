package httpserver

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
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
	Task             string `json:"task"`
	RequirementsPath string `json:"requirementsPath,omitempty"`
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
	})
	mux.HandleFunc("/v1/code/run", s.handleRunForAgent("code"))
	mux.HandleFunc("/v1/analysis/run", s.handleRunForAgent("analysis"))
	mux.HandleFunc("/v1/eval/run", s.handleRunForAgent("eval"))
	mux.HandleFunc("/v1/req/run", s.handleRunForAgent("requirements"))
	mux.HandleFunc("/v1/requirements/run", s.handleRunForAgent("requirements"))
	mux.HandleFunc("/v1/code/chat", s.handleRunForAgent("code"))
	mux.HandleFunc("/v1/analysis/chat", s.handleRunForAgent("analysis"))
	mux.HandleFunc("/v1/eval/chat", s.handleRunForAgent("eval"))
	mux.HandleFunc("/v1/req/chat", s.handleRunForAgent("requirements"))
	mux.HandleFunc("/v1/requirements/chat", s.handleRunForAgent("requirements"))
	mux.HandleFunc("/v1/build/run", s.handleBuildRun())
	// Backward compatible endpoint: defaults to current server agent.
	mux.HandleFunc("/v1/run", s.handleRunForAgent(s.defaultAgent))
	mux.HandleFunc("/v1/chat", s.handleRunForAgent(s.defaultAgent))
	return mux
}

func (s *Server) handleBuildRun() http.HandlerFunc {
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
		svc := core.NewBuildService(s.cfg)
		result, err := svc.RunBuildTask(r.Context(), req.Task, req.RequirementsPath, nil)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
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
		svc, err := core.NewService(r.Context(), s.cfg, agentName)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		result, err := svc.RunTask(r.Context(), req.Task)
		if err != nil {
			writeJSON(w, http.StatusBadRequest, map[string]string{"error": err.Error()})
			return
		}
		writeJSON(w, http.StatusOK, result)
	}
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}
