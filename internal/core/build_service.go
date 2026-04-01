package core

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"build-agent/internal/config"
)

type BuildService struct {
	cfg *config.Config
}

func NewBuildService(cfg *config.Config) *BuildService {
	return &BuildService{cfg: cfg}
}

func (s *BuildService) RunBuildTask(ctx context.Context, task, requirementsPath string, onProgress ProgressFunc) (*RunResult, error) {
	reqAbs, reqRel, err := s.resolveRequirementsPath(requirementsPath)
	if err != nil {
		return nil, err
	}
	if _, statErr := os.Stat(reqAbs); statErr != nil {
		return nil, fmt.Errorf("requirements 文件不可读或不存在：%s；请先完成 req 阶段", reqRel)
	}
	maxRetries := max(1, s.cfg.Base.BuildMaxRetries)

	var finalEvents []EventLog
	appendResult := func(name string, rr *RunResult) {
		finalEvents = append(finalEvents, EventLog{AgentName: "build", Output: fmt.Sprintf("[%s] has_error=%v", name, rr.HasError)})
		finalEvents = append(finalEvents, rr.Events...)
	}

	codeTask := s.buildCodeTask(task, reqRel, "")
	codeSvc, err := NewService(ctx, s.cfg, "code")
	if err != nil {
		return nil, err
	}
	evalSvc, err := NewService(ctx, s.cfg, "eval")
	if err != nil {
		return nil, err
	}

	for i := 0; i < maxRetries; i++ {
		if onProgress != nil {
			onProgress(EventLog{AgentName: "build", Output: fmt.Sprintf("build loop %d/%d: run code", i+1, maxRetries)})
		}
		codeRes, runErr := codeSvc.RunTaskWithProgress(ctx, codeTask, onProgress)
		if runErr != nil {
			return nil, fmt.Errorf("code 阶段失败: %w", runErr)
		}
		appendResult("code", codeRes)

		if onProgress != nil {
			onProgress(EventLog{AgentName: "build", Output: fmt.Sprintf("build loop %d/%d: run eval", i+1, maxRetries)})
		}
		evalTask := s.buildEvalTask(task, reqRel)
		evalRes, runErr := evalSvc.RunTaskWithProgress(ctx, evalTask, onProgress)
		if runErr != nil {
			return nil, fmt.Errorf("eval 阶段失败: %w", runErr)
		}
		appendResult("eval", evalRes)

		evalAbs := s.resolveLatestEvaluationPath(reqRel)
		sum, parseErr := ParseEvaluationSummary(evalAbs)
		if parseErr != nil {
			return nil, fmt.Errorf("解析评测文件失败（%s）: %w", filepath.ToSlash(mustRelPath(s.cfg.Base.WorkspaceRoot, evalAbs)), parseErr)
		}
		if sum.Passed {
			if onProgress != nil {
				onProgress(EventLog{AgentName: "build", Output: "评测通过，开始执行 analysis 更新 design.md"})
			}
			analysisSvc, svcErr := NewService(ctx, s.cfg, "analysis")
			if svcErr != nil {
				return nil, svcErr
			}
			analysisTask := s.buildAnalysisTask(task, reqRel, sum.Score)
			analysisRes, runErr := analysisSvc.RunTaskWithProgress(ctx, analysisTask, onProgress)
			if runErr != nil {
				return nil, fmt.Errorf("analysis 阶段失败: %w", runErr)
			}
			appendResult("analysis", analysisRes)
			return &RunResult{
				Output:   s.buildFinalOutput(true, i+1, maxRetries, reqRel, sum, analysisRes.Output),
				Events:   finalEvents,
				HasError: false,
			}, nil
		}

		if i == maxRetries-1 {
			return &RunResult{
				Output:   s.buildFinalOutput(false, i+1, maxRetries, reqRel, sum, ""),
				Events:   finalEvents,
				HasError: true,
			}, nil
		}
		codeTask = s.buildCodeTask(task, reqRel, sum.FailureItems)
	}
	return nil, fmt.Errorf("build 循环异常结束")
}

func (s *BuildService) resolveRequirementsPath(path string) (string, string, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		if latest := s.findLatestReqPath(); latest != "" {
			path = latest
		} else {
			return "", "", fmt.Errorf("未找到需求文件：请先在 .spec 下生成 REQ-xxxxx.md，或通过 --requirements-path 指定")
		}
	}
	abs, rel, err := configResolvePathUnderRoot(s.cfg.Base.WorkspaceRoot, path)
	if err != nil {
		return "", "", fmt.Errorf("requirementsPath 无效: %w", err)
	}
	return abs, rel, nil
}

func (s *BuildService) buildCodeTask(userTask, reqRel, failureItems string) string {
	base := fmt.Sprintf("用户任务：%s\nrequirementsPath=%s\n请先读取 requirementsPath，再实现代码并执行构建验证。", strings.TrimSpace(userTask), reqRel)
	if strings.TrimSpace(failureItems) == "" {
		return base + "\n若存在 .spec/EVAL-REQ-xxxxx-xx.md，请参考历史不通过项。"
	}
	return base + "\n以下是本轮评测不通过项，请最小改动修复并逐项闭环：\n" + failureItems
}

func (s *BuildService) buildEvalTask(userTask, reqRel string) string {
	return fmt.Sprintf("用户任务：%s\nrequirementsPath=%s\n请对照 requirementsPath 评测当前实现并按规则输出到 .spec/EVAL-REQ-xxxxx-xx.md。", strings.TrimSpace(userTask), reqRel)
}

func (s *BuildService) buildAnalysisTask(userTask, reqRel string, score int) string {
	return fmt.Sprintf("用户任务：%s\nrequirementsPath=%s\n当前评测已通过（score=%d）。请更新 design.md，反映本次实现后的模块行为、配置变化、启动/验证方式与限制。", strings.TrimSpace(userTask), reqRel, score)
}

func (s *BuildService) buildFinalOutput(passed bool, loops, maxRetries int, reqRel string, sum EvaluationSummary, analysisOutput string) string {
	scoreText := "未知"
	if sum.HasScore {
		scoreText = fmt.Sprintf("%d/100", sum.Score)
	}
	if passed {
		return fmt.Sprintf("build 完成：评测通过。\nrequirementsPath=%s\n循环次数=%d/%d\n评分=%s\n\nanalysis 输出：\n%s", reqRel, loops, maxRetries, scoreText, strings.TrimSpace(analysisOutput))
	}
	failure := strings.TrimSpace(sum.FailureItems)
	if failure == "" {
		failure = "EVAL-REQ-xxxxx-xx.md 未提供明确不通过项"
	}
	return fmt.Sprintf("build 失败：达到最大重试次数仍未通过。\nrequirementsPath=%s\n循环次数=%d/%d\n评分=%s\n不通过项：\n%s", reqRel, loops, maxRetries, scoreText, failure)
}

func configResolvePathUnderRoot(rootAbs, userPath string) (abs string, rel string, err error) {
	cleanRoot, err := filepath.Abs(rootAbs)
	if err != nil {
		return "", "", err
	}
	var joined string
	if filepath.IsAbs(userPath) {
		joined = filepath.Clean(userPath)
	} else {
		joined = filepath.Join(cleanRoot, filepath.Clean(userPath))
	}
	absPath, err := filepath.Abs(joined)
	if err != nil {
		return "", "", err
	}
	relPath, err := filepath.Rel(cleanRoot, absPath)
	if err != nil {
		return "", "", err
	}
	if relPath == ".." || strings.HasPrefix(relPath, ".."+string(filepath.Separator)) {
		return "", "", fmt.Errorf("must be inside WORKSPACE_ROOT")
	}
	return absPath, relPath, nil
}

func (s *BuildService) findLatestReqPath() string {
	specAbs := filepath.Join(s.cfg.Base.WorkspaceRoot, ".spec")
	entries, err := os.ReadDir(specAbs)
	if err != nil {
		return ""
	}
	re := regexp.MustCompile(`^REQ-(\d{5})\.md$`)
	type item struct {
		n    int
		name string
	}
	items := make([]item, 0, 8)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		m := re.FindStringSubmatch(e.Name())
		if len(m) < 2 {
			continue
		}
		n := 0
		fmt.Sscanf(m[1], "%d", &n)
		items = append(items, item{n: n, name: e.Name()})
	}
	if len(items) == 0 {
		return ""
	}
	sort.Slice(items, func(i, j int) bool { return items[i].n > items[j].n })
	return filepath.ToSlash(filepath.Join(".spec", items[0].name))
}

func (s *BuildService) resolveLatestEvaluationPath(reqRel string) string {
	reqBase := strings.TrimSuffix(filepath.Base(filepath.ToSlash(reqRel)), filepath.Ext(reqRel))
	if matched, _ := regexp.MatchString(`^REQ-\d{5}$`, reqBase); matched {
		specAbs := filepath.Join(s.cfg.Base.WorkspaceRoot, ".spec")
		entries, err := os.ReadDir(specAbs)
		if err == nil {
			re := regexp.MustCompile(fmt.Sprintf(`^EVAL-%s-(\d{2})\.md$`, regexp.QuoteMeta(reqBase)))
			bestRound := -1
			bestPath := ""
			for _, e := range entries {
				if e.IsDir() {
					continue
				}
				m := re.FindStringSubmatch(e.Name())
				if len(m) < 2 {
					continue
				}
				round := 0
				fmt.Sscanf(m[1], "%d", &round)
				if round > bestRound {
					bestRound = round
					bestPath = filepath.Join(specAbs, e.Name())
				}
			}
			if bestPath != "" {
				return bestPath
			}
		}
	}
	return filepath.Join(s.cfg.Base.WorkspaceRoot, ".spec", fmt.Sprintf("EVAL-%s-01.md", reqBase))
}

func mustRelPath(root, abs string) string {
	rel, err := filepath.Rel(root, abs)
	if err != nil {
		return abs
	}
	return rel
}
