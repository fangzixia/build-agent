package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"
)

// RequirementInfo 需求信息
type RequirementInfo struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Status    string    `json:"status"`
	Score     int       `json:"score,omitempty"`
	Rounds    int       `json:"rounds,omitempty"`
	CreatedAt time.Time `json:"createdAt"`
	Path      string    `json:"path"`
	Content   string    `json:"content,omitempty"`
}

func (s *Server) handleGetRequirements() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		specDir := filepath.Join(s.cfg.Base.WorkspaceRoot, ".spec")
		requirements, err := s.loadRequirements(specDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"requirements": requirements,
		})
	}
}

func (s *Server) loadRequirements(specDir string) ([]RequirementInfo, error) {
	requirements := make([]RequirementInfo, 0)

	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		return requirements, nil
	}

	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil, err
	}

	reqPattern := regexp.MustCompile(`^REQ-(.+)\.md$`)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !reqPattern.MatchString(entry.Name()) {
			continue
		}

		matches := reqPattern.FindStringSubmatch(entry.Name())
		if len(matches) < 2 {
			continue
		}

		reqID := "REQ-" + matches[1]
		reqPath := filepath.Join(specDir, entry.Name())

		content, err := os.ReadFile(reqPath)
		if err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		req := RequirementInfo{
			ID:        reqID,
			Title:     extractTitle(string(content)),
			Status:    "pending",
			CreatedAt: info.ModTime(),
			Path:      filepath.ToSlash(filepath.Join(".spec", entry.Name())),
			Content:   string(content),
		}

		latestEval := s.findLatestEvaluation(specDir, reqID)
		if latestEval != nil {
			req.Score = latestEval.Score
			req.Rounds = latestEval.Round
			if latestEval.Passed {
				req.Status = "passed"
			} else {
				req.Status = "failed"
			}
		}

		requirements = append(requirements, req)
	}

	sort.Slice(requirements, func(i, j int) bool {
		return requirements[i].ID > requirements[j].ID
	})

	return requirements, nil
}

func extractTitle(content string) string {
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if title, found := strings.CutPrefix(line, "# "); found {
			return title
		}
	}
	return "未命名需求"
}
