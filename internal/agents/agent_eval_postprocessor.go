package agents

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

type evalPostProcessor struct {
	workspaceRoot string
	passThreshold int
}

func (p evalPostProcessor) Process(in PostProcessInput) PostProcessResult {
	const draftRel = ".eval-agent-tmp/evaluation-draft.md"
	draftAbs := filepath.Join(p.workspaceRoot, draftRel)
	finalRel := p.resolveEvalOutputRel(in)
	finalAbs := filepath.Join(p.workspaceRoot, filepath.FromSlash(finalRel))
	_ = os.MkdirAll(filepath.Dir(finalAbs), 0o755)
	draftBytes, err := os.ReadFile(draftAbs)
	draftOK := err == nil && strings.TrimSpace(string(draftBytes)) != ""
	score, hasScore := extractScore(string(draftBytes))
	evaluatedAtTime := time.Now().In(time.Local)
	evaluatedAt := formatEvaluationTimestamp(evaluatedAtTime)

	var body string
	if draftOK && !in.HasError {
		body = prependEvaluationSummary(string(draftBytes), p.passThreshold, evaluatedAtTime)
	} else {
		body = buildAutoEvaluationMarkdown(in.HasError, draftOK, hasScore, score, p.passThreshold, evaluatedAtTime, in.Output, in.Events, string(draftBytes))
	}
	out := PostProcessResult{
		Output:      in.Output,
		HasError:    in.HasError,
		EvaluatedAt: evaluatedAt,
	}
	if writeErr := os.WriteFile(finalAbs, []byte(body), 0o644); writeErr == nil {
		out.Progress = append(out.Progress, PostProcessEvent{AgentName: "system", Output: "评测报告已保存为 " + finalRel + "；评测时间：" + evaluatedAt})
		out.Output = strings.TrimRight(out.Output, "\n") + "\n\n评测报告文件（相对工作区）: " + finalRel
	}
	return out
}

func (p evalPostProcessor) resolveEvalOutputRel(in PostProcessInput) string {
	reqID := extractReqIDFromPostInput(in)
	round := p.nextEvalRound(reqID)
	return fmt.Sprintf(".spec/EVAL-%s-%02d.md", reqID, round)
}

func extractReqIDFromPostInput(in PostProcessInput) string {
	reReqID := regexp.MustCompile(`\bREQ-(\d{5})\b`)
	if m := reReqID.FindStringSubmatch(in.Output); len(m) >= 2 {
		return "REQ-" + m[1]
	}
	for _, ev := range in.Events {
		if m := reReqID.FindStringSubmatch(ev.Output); len(m) >= 2 {
			return "REQ-" + m[1]
		}
	}
	return "REQ-00000"
}

func (p evalPostProcessor) nextEvalRound(reqID string) int {
	specAbs := filepath.Join(p.workspaceRoot, ".spec")
	entries, err := os.ReadDir(specAbs)
	if err != nil {
		return 1
	}
	re := regexp.MustCompile(fmt.Sprintf(`^EVAL-%s-(\d{2})\.md$`, regexp.QuoteMeta(reqID)))
	var rounds []int
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(e.Name())
		if len(m) < 2 {
			continue
		}
		n, convErr := strconv.Atoi(m[1])
		if convErr == nil && n > 0 && n < 100 {
			rounds = append(rounds, n)
		}
	}
	if len(rounds) == 0 {
		return 1
	}
	sort.Ints(rounds)
	next := rounds[len(rounds)-1] + 1
	if next > 99 {
		return 99
	}
	return next
}

func formatEvaluationTimestamp(t time.Time) string {
	t = t.In(time.Local)
	return fmt.Sprintf("%d年%d月%d日 %d时%02d分%02d秒", t.Year(), t.Month(), t.Day(), t.Hour(), t.Minute(), t.Second())
}

func prependEvaluationSummary(draft string, passThreshold int, evaluatedAt time.Time) string {
	score, ok := extractScore(draft)
	pass := "见正文"
	scoreLine := "见正文"
	passed := false
	if ok {
		scoreLine = strconv.Itoa(score) + "/100"
		if score >= passThreshold {
			pass = "是"
			passed = true
		} else {
			pass = "否"
		}
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 评测结果\n\n")
	fmt.Fprintf(&b, "| 项目 | 结果 |\n|------|------|\n")
	fmt.Fprintf(&b, "| **评测时间** | **%s** （本地时间） |\n", formatEvaluationTimestamp(evaluatedAt))
	fmt.Fprintf(&b, "| **吻合度评分** | **%s** |\n", scoreLine)
	fmt.Fprintf(&b, "| **是否通过评测** | **%s** （≥%d 分视为通过） |\n\n", pass, passThreshold)
	if !passed {
		fmt.Fprintf(&b, "## 不通过项说明\n\n")
		if items := extractFailureItemsFromDraft(draft); strings.TrimSpace(items) != "" {
			b.WriteString(items + "\n\n")
		} else if ok {
			fmt.Fprintf(&b, "当前得分 **%d** 低于通过阈值 **%d**；草稿中未解析到明确不通过条目，请见正文证据。\n\n", score, passThreshold)
		} else {
			b.WriteString("未能从草稿中解析数值分数；请参考正文中的需求对照项。\n\n")
		}
	}
	b.WriteString("---\n\n")
	b.WriteString(draft)
	return b.String()
}

func extractFailureItemsFromDraft(draft string) string {
	// Go regexp(RE2) does not support lookahead, so parse H2 sections directly.
	lines := strings.Split(draft, "\n")
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
		if len(body) > 10 {
			return body
		}
	}
	neg := regexp.MustCompile(`(?i)(未通过|不满足|不通过|缺失|失败|×|✗|部分满足)`)
	negLines := make([]string, 0, 8)
	for _, line := range lines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if neg.MatchString(t) && (strings.HasPrefix(t, "-") || strings.HasPrefix(t, "*") || strings.HasPrefix(t, "|")) {
			negLines = append(negLines, t)
		}
		if len(negLines) >= 20 {
			break
		}
	}
	return strings.Join(negLines, "\n")
}

func buildAutoEvaluationMarkdown(hasError, draftOK, hasScore bool, score, passThreshold int, evaluatedAt time.Time, finalOutput string, events []PostProcessEvent, draft string) string {
	if !hasScore {
		score = 0
	}
	var b strings.Builder
	fmt.Fprintf(&b, "# 评测结果\n\n")
	fmt.Fprintf(&b, "| 项目 | 结果 |\n|------|------|\n")
	fmt.Fprintf(&b, "| **评测时间** | **%s** （本地时间） |\n", formatEvaluationTimestamp(evaluatedAt))
	fmt.Fprintf(&b, "| **吻合度评分** | **%d/100** |\n", score)
	pass := "否"
	if hasScore && score >= passThreshold {
		pass = "是"
	}
	fmt.Fprintf(&b, "| **是否通过评测** | **%s** （≥%d 分视为通过） |\n\n", pass, passThreshold)
	fmt.Fprintf(&b, "## 不通过项说明\n\n")
	if pass == "否" {
		fmt.Fprintf(&b, "- 综合得分 **%d** 分，低于通过阈值 **%d** 分。\n", score, passThreshold)
	}
	switch {
	case hasError && !draftOK:
		fmt.Fprintf(&b, "- 自动化流程未正常结束，且未在 `%s` 生成有效草稿。\n", ".eval-agent-tmp/evaluation-draft.md")
	case hasError && draftOK:
		fmt.Fprintf(&b, "- 自动化流程未正常结束；以下摘录模型草稿中可能涉及的不通过项。\n")
	case !hasError && !draftOK:
		fmt.Fprintf(&b, "- 未检测到有效评测草稿，无法分项对照需求。\n")
	}
	if items := formatFailureItemsFromEvents(events); items != "" {
		b.WriteString("\n" + items + "\n")
	}
	if draftOK && strings.TrimSpace(draft) != "" {
		if ext := extractFailureItemsFromDraft(draft); ext != "" {
			fmt.Fprintf(&b, "\n**自草稿提取的分项说明**：\n\n%s\n", ext)
		}
		fmt.Fprintf(&b, "\n## 草稿附录\n\n```\n%s\n```\n", truncate(draft, 4000))
	}
	if strings.TrimSpace(finalOutput) != "" {
		fmt.Fprintf(&b, "\n**模型输出摘录**：\n\n```\n%s\n```\n", truncate(finalOutput, 1200))
	}
	fmt.Fprintf(&b, "\n**事件摘录**（最多 8 条）：\n\n")
	for i, ev := range events {
		if i >= 8 {
			break
		}
		parts := []string{ev.AgentName}
		if ev.ToolName != "" {
			parts = append(parts, ev.ToolName)
		}
		if ev.Error != "" {
			parts = append(parts, truncate(ev.Error, 120))
		} else if ev.Output != "" {
			parts = append(parts, truncate(ev.Output, 80))
		}
		fmt.Fprintf(&b, "- %s\n", strings.Join(parts, " | "))
	}
	return b.String()
}

func formatFailureItemsFromEvents(events []PostProcessEvent) string {
	var b strings.Builder
	n := 0
	for _, ev := range events {
		if ev.Error == "" {
			continue
		}
		n++
		fmt.Fprintf(&b, "- **[%s]** %s\n", ev.AgentName, truncate(ev.Error, 400))
		if n >= 12 {
			fmt.Fprintf(&b, "- …（其余错误见事件摘录）\n")
			break
		}
	}
	for _, line := range collectRunCommandFailures(events) {
		if n >= 15 {
			break
		}
		fmt.Fprintf(&b, "- %s\n", line)
		n++
	}
	return strings.TrimSpace(b.String())
}

func collectRunCommandFailures(events []PostProcessEvent) []string {
	out := make([]string, 0, 4)
	for _, ev := range events {
		if ev.ToolName != "run_command" || strings.TrimSpace(ev.Output) == "" {
			continue
		}
		var obj map[string]any
		if err := json.Unmarshal([]byte(ev.Output), &obj); err != nil {
			continue
		}
		code := 0
		switch v := obj["exit_code"].(type) {
		case float64:
			code = int(v)
		case int:
			code = v
		}
		if code == 0 {
			continue
		}
		cmd := fmt.Sprint(obj["command"])
		stderr := truncate(fmt.Sprint(obj["stderr"]), 200)
		out = append(out, fmt.Sprintf("**run_command** 非零退出（exit=%d）：`%s` — %s", code, truncate(cmd, 120), stderr))
	}
	return out
}

func extractScore(s string) (int, bool) {
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

func truncate(s string, max int) string {
	s = strings.TrimSpace(s)
	if len(s) <= max {
		return s
	}
	return s[:max] + "\n...(已截断)"
}
