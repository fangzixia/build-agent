package core

import (
	"os"
	"regexp"
	"strconv"
	"strings"
)

type EvaluationSummary struct {
	Passed       bool
	HasPassField bool
	Score        int
	HasScore     bool
	FailureItems string
}

func ParseEvaluationSummary(path string) (EvaluationSummary, error) {
	b, err := os.ReadFile(path)
	if err != nil {
		return EvaluationSummary{}, err
	}
	text := string(b)
	score, hasScore := extractEvaluationScore(text)
	passed, hasPass := extractEvaluationPass(text)
	failures := extractEvaluationFailureItems(text)
	if !hasPass && hasScore {
		passed = score >= 80
	}
	return EvaluationSummary{
		Passed:       passed,
		HasPassField: hasPass,
		Score:        score,
		HasScore:     hasScore,
		FailureItems: failures,
	}, nil
}

func extractEvaluationScore(s string) (int, bool) {
	patterns := []*regexp.Regexp{
		regexp.MustCompile(`(?i)吻合度评分[^0-9]{0,60}(\d{1,3})`),
		regexp.MustCompile(`(?i)\*\*(\d{1,3})\*\*\s*/\s*100`),
		regexp.MustCompile(`(\d{1,3})\s*/\s*100`),
	}
	for _, re := range patterns {
		m := re.FindStringSubmatch(s)
		if len(m) < 2 {
			continue
		}
		n, err := strconv.Atoi(m[1])
		if err != nil || n < 0 || n > 100 {
			continue
		}
		return n, true
	}
	return 0, false
}

func extractEvaluationPass(s string) (bool, bool) {
	re := regexp.MustCompile(`(?i)是否通过评测[^|\n\r]*\|[^|\n\r]*\|\s*\**\s*(是|否)\s*\**`)
	if m := re.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1]) == "是", true
	}
	reLine := regexp.MustCompile(`(?i)是否通过评测[^是否]*(是|否)`)
	if m := reLine.FindStringSubmatch(s); len(m) >= 2 {
		return strings.TrimSpace(m[1]) == "是", true
	}
	return false, false
}

func extractEvaluationFailureItems(s string) string {
	lines := strings.Split(s, "\n")
	reBadTitle := regexp.MustCompile(`(?i)(不通过|未通过|差距|问题项|未满足|风险)`)
	for i := 0; i < len(lines); i++ {
		title := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(title, "##") || strings.HasPrefix(title, "###") {
			continue
		}
		if !reBadTitle.MatchString(title) {
			continue
		}
		start := i + 1
		end := len(lines)
		for j := start; j < len(lines); j++ {
			next := strings.TrimSpace(lines[j])
			if strings.HasPrefix(next, "##") && !strings.HasPrefix(next, "###") {
				end = j
				break
			}
		}
		body := strings.TrimSpace(strings.Join(lines[start:end], "\n"))
		if body != "" {
			return body
		}
	}
	return ""
}
