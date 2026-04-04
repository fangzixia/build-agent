package httpserver

import (
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// EvaluationInfo 验收信息
type EvaluationInfo struct {
	RequirementID string        `json:"requirementId"`
	Round         int           `json:"round"`
	Score         int           `json:"score"`
	Passed        bool          `json:"passed"`
	EvaluatedAt   time.Time     `json:"evaluatedAt"`
	Summary       string        `json:"summary"`
	FailedItems   []FailureItem `json:"failedItems,omitempty"`
	Path          string        `json:"path"`
}

// FailureItem 失败项
type FailureItem struct {
	Category string `json:"category"`
	Reason   string `json:"reason"`
}

func (s *Server) handleGetEvaluations() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			writeJSON(w, http.StatusMethodNotAllowed, map[string]string{"error": "method not allowed"})
			return
		}

		specDir := filepath.Join(s.cfg.Base.WorkspaceRoot, ".spec")
		evaluations, err := s.loadEvaluations(specDir)
		if err != nil {
			writeJSON(w, http.StatusInternalServerError, map[string]string{"error": err.Error()})
			return
		}

		writeJSON(w, http.StatusOK, map[string]any{
			"evaluations": evaluations,
		})
	}
}

func (s *Server) loadEvaluations(specDir string) ([]EvaluationInfo, error) {
	evaluations := make([]EvaluationInfo, 0)

	if _, err := os.Stat(specDir); os.IsNotExist(err) {
		return evaluations, nil
	}

	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil, err
	}

	evalPattern := regexp.MustCompile(`^EVAL-(.+)\.md$`)

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		if !evalPattern.MatchString(entry.Name()) {
			continue
		}

		matches := evalPattern.FindStringSubmatch(entry.Name())
		if len(matches) < 2 {
			continue
		}

		evalInfo := matches[1]
		evalPath := filepath.Join(specDir, entry.Name())

		content, err := os.ReadFile(evalPath)
		if err != nil {
			continue
		}

		info, err := entry.Info()
		if err != nil {
			continue
		}

		reqID := ""
		round := 0

		reqRoundPattern := regexp.MustCompile(`^(REQ-.+)-(\d+)$`)
		if reqRoundMatches := reqRoundPattern.FindStringSubmatch(evalInfo); len(reqRoundMatches) >= 3 {
			reqID = reqRoundMatches[1]
			round, _ = strconv.Atoi(reqRoundMatches[2])
		} else {
			reqID = evalInfo
			round = 1
		}

		eval := EvaluationInfo{
			RequirementID: reqID,
			Round:         round,
			EvaluatedAt:   info.ModTime(),
			Path:          filepath.ToSlash(filepath.Join(".spec", entry.Name())),
		}

		score, hasScore := extractScore(string(content))
		if hasScore {
			eval.Score = score
		}

		passed, hasPassed := extractPassed(string(content))
		if hasPassed {
			eval.Passed = passed
		} else if hasScore {
			eval.Passed = score >= 80
		}

		eval.Summary = extractSummary(string(content))
		eval.FailedItems = extractFailedItems(string(content))

		evaluations = append(evaluations, eval)
	}

	sort.Slice(evaluations, func(i, j int) bool {
		return evaluations[i].EvaluatedAt.After(evaluations[j].EvaluatedAt)
	})

	return evaluations, nil
}

func (s *Server) findLatestEvaluation(specDir, reqID string) *EvaluationInfo {
	entries, err := os.ReadDir(specDir)
	if err != nil {
		return nil
	}

	evalPattern := regexp.MustCompile(`^EVAL-` + regexp.QuoteMeta(reqID) + `-(\d+)\.md$`)
	maxRound := -1
	var latestEval *EvaluationInfo

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		matches := evalPattern.FindStringSubmatch(entry.Name())
		if len(matches) < 2 {
			continue
		}

		round, _ := strconv.Atoi(matches[1])
		if round > maxRound {
			maxRound = round
			evalPath := filepath.Join(specDir, entry.Name())
			content, err := os.ReadFile(evalPath)
			if err != nil {
				continue
			}

			info, err := entry.Info()
			if err != nil {
				continue
			}

			eval := &EvaluationInfo{
				RequirementID: reqID,
				Round:         round,
				EvaluatedAt:   info.ModTime(),
				Path:          filepath.ToSlash(filepath.Join(".spec", entry.Name())),
			}

			score, hasScore := extractScore(string(content))
			if hasScore {
				eval.Score = score
			}

			passed, hasPassed := extractPassed(string(content))
			if hasPassed {
				eval.Passed = passed
			} else if hasScore {
				eval.Passed = score >= 80
			}

			latestEval = eval
		}
	}

	return latestEval
}

func extractScore(content string) (int, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)吻合度评分[^0-9]{0,60}(\d{1,3})`),
		regexp.MustCompile(`(?i)\*\*(\d{1,3})\*\*\s*/\s*100`),
		regexp.MustCompile(`(\d{1,3})\s*/\s*100`),
	}

	for _, pattern := range patterns {
		matches := pattern.FindStringSubmatch(content)
		if len(matches) >= 2 {
			score, err := strconv.Atoi(matches[1])
			if err == nil && score >= 0 && score <= 100 {
				return score, true
			}
		}
	}

	return 0, false
}

func extractPassed(content string) (bool, bool) {
	pattern := regexp.MustCompile(`(?i)是否通过评测[^|\n\r]*\|[^|\n\r]*\|\s*\**\s*(是|否)\s*\**`)
	matches := pattern.FindStringSubmatch(content)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1]) == "是", true
	}

	linePattern := regexp.MustCompile(`(?i)是否通过评测[^是否]*(是|否)`)
	matches = linePattern.FindStringSubmatch(content)
	if len(matches) >= 2 {
		return strings.TrimSpace(matches[1]) == "是", true
	}

	return false, false
}

func extractSummary(content string) string {
	lines := strings.Split(content, "\n")
	inSummary := false
	summaryLines := make([]string, 0)

	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "不通过项说明") || strings.Contains(line, "评测摘要") {
			inSummary = true
			continue
		}

		if inSummary && strings.HasPrefix(line, "##") {
			break
		}

		if inSummary && line != "" && !strings.HasPrefix(line, "#") {
			summaryLines = append(summaryLines, line)
			if len(summaryLines) >= 3 {
				break
			}
		}
	}

	if len(summaryLines) > 0 {
		return strings.Join(summaryLines, " ")
	}

	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line != "" && !strings.HasPrefix(line, "#") && !strings.HasPrefix(line, "|") {
			if len(line) > 100 {
				return line[:100] + "..."
			}
			return line
		}
	}

	return "无摘要"
}

func extractFailedItems(content string) []FailureItem {
	items := make([]FailureItem, 0)
	lines := strings.Split(content, "\n")

	inFailureSection := false
	for _, line := range lines {
		line = strings.TrimSpace(line)

		if strings.Contains(line, "不通过项") || strings.Contains(line, "失败项") || strings.Contains(line, "未通过") {
			inFailureSection = true
			continue
		}

		if inFailureSection && strings.HasPrefix(line, "##") && !strings.Contains(line, "不通过") && !strings.Contains(line, "失败") {
			break
		}

		if inFailureSection && (strings.HasPrefix(line, "- ") || strings.HasPrefix(line, "* ")) {
			reason := strings.TrimPrefix(strings.TrimPrefix(line, "- "), "* ")
			category := categorizeFailure(reason)
			items = append(items, FailureItem{
				Category: category,
				Reason:   reason,
			})
		}
	}

	return items
}

func categorizeFailure(reason string) string {
	reason = strings.ToLower(reason)

	if strings.Contains(reason, "编译") || strings.Contains(reason, "语法") || strings.Contains(reason, "错误") {
		return "blocking"
	}
	if strings.Contains(reason, "契约") || strings.Contains(reason, "接口") || strings.Contains(reason, "api") {
		return "contract"
	}
	if strings.Contains(reason, "用户") || strings.Contains(reason, "体验") || strings.Contains(reason, "提示") || strings.Contains(reason, "交互") {
		return "ux"
	}
	if strings.Contains(reason, "边缘") || strings.Contains(reason, "极端") || strings.Contains(reason, "异常") {
		return "edge_case"
	}

	return "other"
}
