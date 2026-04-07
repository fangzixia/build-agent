package config

import (
	"bufio"
	"fmt"
	"strings"
)

// EnvEntry 表示单个环境变量配置项
type EnvEntry struct {
	Key     string `json:"key"`
	Value   string `json:"value"`
	Comment string `json:"comment,omitempty"`
	Section string `json:"section"`
}

// EnvConfig 表示完整的环境配置
type EnvConfig struct {
	Sections map[string][]EnvEntry `json:"sections"`
}

// EnvParser 负责解析和序列化 .env 文件
type EnvParser struct{}

// NewEnvParser 创建新的 ENV 解析器
func NewEnvParser() *EnvParser {
	return &EnvParser{}
}

// Parse 解析 .env 文件内容为结构化数据
func (p *EnvParser) Parse(content string) (*EnvConfig, error) {
	config := &EnvConfig{
		Sections: make(map[string][]EnvEntry),
	}

	scanner := bufio.NewScanner(strings.NewReader(content))
	currentSection := "Base"
	var currentComment string

	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		// 跳过空行
		if line == "" {
			currentComment = ""
			continue
		}

		// 处理注释行
		if strings.HasPrefix(line, "#") {
			comment := strings.TrimSpace(strings.TrimPrefix(line, "#"))
			// 检测区块注释
			if p.isSectionComment(comment) {
				currentSection = p.extractSection(comment)
			} else {
				currentComment = comment
			}
			continue
		}

		// 解析键值对
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			continue
		}

		key := strings.TrimSpace(parts[0])
		value := strings.TrimSpace(parts[1])

		// 根据键名推断区块
		if currentSection == "Base" {
			currentSection = p.inferSection(key)
		}

		entry := EnvEntry{
			Key:     key,
			Value:   value,
			Comment: currentComment,
			Section: currentSection,
		}

		config.Sections[currentSection] = append(config.Sections[currentSection], entry)
		currentComment = ""
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("failed to parse env file: %w", err)
	}

	return config, nil
}

// Serialize 将结构化数据序列化为 .env 格式
func (p *EnvParser) Serialize(config *EnvConfig) (string, error) {
	var builder strings.Builder

	// 定义区块顺序
	sectionOrder := []string{"Base", "CODE", "ANALYSIS", "EVAL", "REQUIREMENTS", "BUILD"}

	for i, sectionName := range sectionOrder {
		entries, exists := config.Sections[sectionName]
		if !exists || len(entries) == 0 {
			continue
		}

		// 添加区块注释
		if i > 0 {
			builder.WriteString("\n")
		}
		if sectionName != "Base" {
			builder.WriteString(fmt.Sprintf("# %s scenario (OPENAI fully isolated)\n", strings.ToLower(sectionName)))
		}

		// 写入配置项
		for _, entry := range entries {
			if entry.Comment != "" && !p.isSectionComment(entry.Comment) {
				builder.WriteString(fmt.Sprintf("# %s\n", entry.Comment))
			}
			builder.WriteString(fmt.Sprintf("%s=%s\n", entry.Key, entry.Value))
		}
	}

	return builder.String(), nil
}

// isSectionComment 检查是否为区块注释
func (p *EnvParser) isSectionComment(comment string) bool {
	lower := strings.ToLower(comment)
	return strings.Contains(lower, "scenario") ||
		strings.Contains(lower, "agent") ||
		strings.Contains(lower, "orchestrates")
}

// extractSection 从注释中提取区块名称
func (p *EnvParser) extractSection(comment string) string {
	lower := strings.ToLower(comment)

	if strings.Contains(lower, "code") {
		return "CODE"
	}
	if strings.Contains(lower, "analysis") {
		return "ANALYSIS"
	}
	if strings.Contains(lower, "eval") {
		return "EVAL"
	}
	if strings.Contains(lower, "requirements") {
		return "REQUIREMENTS"
	}
	if strings.Contains(lower, "build") {
		return "BUILD"
	}

	return "Base"
}

// inferSection 根据键名推断区块
func (p *EnvParser) inferSection(key string) string {
	upper := strings.ToUpper(key)

	if strings.HasPrefix(upper, "CODE_") {
		return "CODE"
	}
	if strings.HasPrefix(upper, "ANALYSIS_") {
		return "ANALYSIS"
	}
	if strings.HasPrefix(upper, "EVAL_") {
		return "EVAL"
	}
	if strings.HasPrefix(upper, "REQUIREMENTS_") {
		return "REQUIREMENTS"
	}
	if strings.HasPrefix(upper, "BUILD_") {
		return "BUILD"
	}

	return "Base"
}
