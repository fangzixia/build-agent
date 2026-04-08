package core

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"unicode/utf8"

	einomodel "github.com/cloudwego/eino/components/model"
	"github.com/cloudwego/eino/schema"
)

const (
	defaultMaxContextTokens       = 120000
	defaultSmartCompressThreshold = 90000
	// 保守估算：1 token ≈ 2 个 UTF-8 字符（中英混合场景，中文约 1.5~2 字符/token）
	charsPerToken = 2
	// 压缩后保留的安全余量（token 数）
	safetyMargin = 5000
)

// CompressNotify 用于向用户反馈压缩过程
type CompressNotify func(msg string)

// contextCompressor 负责在 GenInputFn 中检测并压缩超长上下文
type contextCompressor struct {
	maxTokens      int
	smartThreshold int
	model          einomodel.BaseChatModel // 用于 LLM 智能压缩，可为 nil
	notify         CompressNotify
}

func newContextCompressor(maxTokens, smartThreshold int, model einomodel.BaseChatModel, notify CompressNotify) *contextCompressor {
	if maxTokens <= 0 {
		maxTokens = defaultMaxContextTokens
	}
	if smartThreshold <= 0 || smartThreshold >= maxTokens {
		smartThreshold = maxTokens * 77 / 100 // 默认约 77% 处触发
	}
	return &contextCompressor{
		maxTokens:      maxTokens,
		smartThreshold: smartThreshold,
		model:          model,
		notify:         notify,
	}
}

// estimateTokens 粗估字符串的 token 数
func estimateTokens(s string) int {
	return (utf8.RuneCountInString(s) + charsPerToken - 1) / charsPerToken
}

// CompressExecutedSteps 检测并压缩 executedJSON。
// 两段式策略：
//   - 总 token 数超过 smartThreshold → 触发 LLM 智能压缩（预防性）
//   - 总 token 数超过 maxTokens      → 降级到机械截断（兜底）
func (c *contextCompressor) CompressExecutedSteps(ctx context.Context, systemPrompt, taskPrompt, executedJSON string) (string, bool) {
	baseTokens := estimateTokens(systemPrompt) + estimateTokens(taskPrompt)
	execTokens := estimateTokens(executedJSON)
	total := baseTokens + execTokens

	// 未达到任何阈值，直接返回
	if total <= c.smartThreshold {
		return executedJSON, false
	}

	// 达到智能压缩阈值但未超硬上限：尝试 LLM 压缩
	if total <= c.maxTokens && c.model != nil {
		compressed, ok := c.llmCompress(ctx, executedJSON, c.maxTokens-safetyMargin-baseTokens)
		if ok {
			return compressed, true
		}
		// LLM 压缩失败，继续走机械压缩
	}

	// 超过硬上限或 LLM 不可用：机械压缩
	return c.mechanicalCompress(systemPrompt, taskPrompt, executedJSON, baseTokens)
}

// llmCompress 调用 LLM 对 executedJSON 做语义压缩，返回压缩后的文本和是否成功
func (c *contextCompressor) llmCompress(ctx context.Context, executedJSON string, budgetTokens int) (string, bool) {
	c.emitNotify("🤖 上下文接近上限，正在使用 LLM 智能压缩执行历史...")

	// 确保发给 LLM 的内容本身不超限（留 20k token 给 prompt overhead）
	const llmOverhead = 20000
	maxInputChars := (budgetTokens - llmOverhead) * charsPerToken
	inputJSON := executedJSON
	if utf8.RuneCountInString(inputJSON) > maxInputChars && maxInputChars > 0 {
		runes := []rune(inputJSON)
		inputJSON = string(runes[:maxInputChars]) + "...[pre-truncated for LLM]"
	}

	prompt := fmt.Sprintf(`你是一个执行历史压缩助手。以下是一个 AI agent 的执行步骤记录（JSON 格式）。

请将其压缩为简洁的文字摘要，保留以下关键信息：
1. 每个步骤的目的和结果（成功/失败）
2. 关键的文件路径、命令、错误信息
3. 对后续执行有影响的重要发现

要求：
- 输出纯文本摘要，不需要 JSON 格式
- 尽量简洁，控制在 %d token 以内
- 保留足够信息让后续步骤能继续执行

执行历史：
%s`, budgetTokens/2, inputJSON)

	msgs := []*schema.Message{schema.UserMessage(prompt)}
	resp, err := c.model.Generate(ctx, msgs)
	if err != nil {
		c.emitNotify(fmt.Sprintf("⚠️ LLM 智能压缩失败（%v），降级到机械压缩", err))
		return "", false
	}

	summary := strings.TrimSpace(resp.Content)
	if summary == "" {
		c.emitNotify("⚠️ LLM 智能压缩返回空内容，降级到机械压缩")
		return "", false
	}

	// 校验压缩结果是否满足 budget
	if estimateTokens(summary) > budgetTokens {
		c.emitNotify("⚠️ LLM 压缩结果仍超限，降级到机械压缩")
		return "", false
	}

	c.emitNotify(fmt.Sprintf("✅ LLM 智能压缩完成（%d → %d tokens）", estimateTokens(executedJSON), estimateTokens(summary)))
	// 包装成可识别的格式，让 executor 知道这是摘要
	return fmt.Sprintf(`[{"Step":"[LLM压缩摘要]","Result":%s}]`, jsonString(summary)), true
}

// mechanicalCompress 机械分级压缩，作为兜底
func (c *contextCompressor) mechanicalCompress(systemPrompt, taskPrompt, executedJSON string, baseTokens int) (string, bool) {
	limit := c.maxTokens - safetyMargin
	budget := limit - baseTokens
	if budget <= 0 {
		c.emitNotify("⚠️ 上下文超限（system+task 已超限），已清空执行历史")
		return "[]", true
	}

	var steps []map[string]any
	if err := json.Unmarshal([]byte(executedJSON), &steps); err != nil || len(steps) == 0 {
		maxChars := budget * charsPerToken
		if utf8.RuneCountInString(executedJSON) > maxChars {
			runes := []rune(executedJSON)
			c.emitNotify("⚠️ 上下文超限，已截断执行历史 JSON")
			return string(runes[:maxChars]) + "...]", true
		}
		return executedJSON, false
	}

	// Level 0：按 budget 动态截断每步 Result
	budgetChars := budget * charsPerToken
	perStepBudget := budgetChars
	if len(steps) > 0 {
		perStepBudget = budgetChars / len(steps)
	}
	if perStepBudget < 200 {
		perStepBudget = 200
	}
	compressed := truncateStepOutputs(steps, perStepBudget)
	j, _ := json.Marshal(compressed)
	if estimateTokens(string(j)) <= budget {
		c.emitNotify(fmt.Sprintf("⚠️ 上下文超限，已截断工具输出（每步最多 %d 字符，共 %d 步）", perStepBudget, len(steps)))
		return string(j), true
	}

	// Level 1：固定截断到 500 字符
	compressed = truncateStepOutputs(steps, 500)
	j, _ = json.Marshal(compressed)
	if estimateTokens(string(j)) <= budget {
		c.emitNotify(fmt.Sprintf("⚠️ 上下文超限，已压缩执行历史（截断至 500 字符，共 %d 步）", len(steps)))
		return string(j), true
	}

	// Level 2：只保留最近 10 步
	const keepRecent = 10
	if len(steps) > keepRecent {
		dropped := len(steps) - keepRecent
		steps = steps[dropped:]
		compressed = truncateStepOutputs(steps, 300)
		j, _ = json.Marshal(compressed)
		if estimateTokens(string(j)) <= budget {
			c.emitNotify(fmt.Sprintf("⚠️ 上下文超限，保留最近 %d 步，省略前 %d 步", keepRecent, dropped))
			return string(j), true
		}
	}

	// Level 3：只保留步骤摘要
	summaries := summarizeSteps(steps)
	j, _ = json.Marshal(summaries)
	if estimateTokens(string(j)) <= budget {
		c.emitNotify(fmt.Sprintf("⚠️ 上下文严重超限，已压缩为步骤摘要（%d 步）", len(steps)))
		return string(j), true
	}

	c.emitNotify("⚠️ 上下文严重超限，已清空执行历史以继续执行")
	return "[]", true
}

func (c *contextCompressor) emitNotify(msg string) {
	if c.notify != nil {
		c.notify(msg)
	}
}

// jsonString 将字符串安全地序列化为 JSON 字符串字面量
func jsonString(s string) string {
	b, _ := json.Marshal(s)
	return string(b)
}

// truncateStepOutputs 截断每步的 output/result 字段（大小写不敏感）
func truncateStepOutputs(steps []map[string]any, maxChars int) []map[string]any {
	result := make([]map[string]any, len(steps))
	for i, step := range steps {
		copied := make(map[string]any, len(step))
		for k, v := range step {
			kl := strings.ToLower(k)
			if kl == "output" || kl == "result" || kl == "content" {
				if s, ok := v.(string); ok && utf8.RuneCountInString(s) > maxChars {
					runes := []rune(s)
					copied[k] = string(runes[:maxChars]) + "...[truncated]"
					continue
				}
			}
			copied[k] = v
		}
		result[i] = copied
	}
	return result
}

// summarizeSteps 只保留步骤的关键字段（大小写不敏感）
func summarizeSteps(steps []map[string]any) []map[string]any {
	result := make([]map[string]any, len(steps))
	keepKeys := map[string]bool{
		"step": true, "step_name": true, "name": true,
		"status": true, "error": true, "type": true, "tool": true,
	}
	for i, step := range steps {
		summary := make(map[string]any)
		for k, v := range step {
			if keepKeys[strings.ToLower(k)] {
				summary[k] = v
			}
		}
		result[i] = summary
	}
	return result
}
