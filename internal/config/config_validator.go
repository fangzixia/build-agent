package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// ValidationError 表示验证错误
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ConfigValidator 负责验证配置项
type ConfigValidator struct {
	workspaceRoot string
}

// NewConfigValidator 创建新的配置验证器
func NewConfigValidator(workspaceRoot string) *ConfigValidator {
	return &ConfigValidator{
		workspaceRoot: workspaceRoot,
	}
}

// Validate 验证完整配置
func (v *ConfigValidator) Validate(config *EnvConfig) []ValidationError {
	var errors []ValidationError

	// 验证所有配置项
	for _, entries := range config.Sections {
		for _, entry := range entries {
			if err := v.ValidateEntry(entry.Key, entry.Value); err != nil {
				errors = append(errors, ValidationError{
					Field:   entry.Key,
					Message: err.Error(),
				})
			}
		}
	}

	return errors
}

// ValidateEntry 验证单个配置项
func (v *ConfigValidator) ValidateEntry(key, value string) error {
	// 跳过空值验证（某些字段可能为空）
	if value == "" {
		// 检查是否为必填字段
		if v.isRequired(key) {
			return fmt.Errorf("field is required")
		}
		return nil
	}

	upper := strings.ToUpper(key)

	// 验证路径字段
	if strings.Contains(upper, "PATH") || strings.Contains(upper, "DIR") || upper == "WORKSPACE_ROOT" {
		return v.validatePath(value)
	}

	// 验证 URL 字段
	if strings.Contains(upper, "BASE_URL") {
		return v.validateURL(value)
	}

	// 验证 API Key
	if strings.Contains(upper, "API_KEY") {
		return v.validateAPIKey(value)
	}

	// 验证模型名称
	if strings.Contains(upper, "MODEL") {
		return v.validateModel(value)
	}

	// 验证迭代次数
	if strings.Contains(upper, "ITERATIONS") || strings.Contains(upper, "RETRIES") {
		return v.validatePositiveInt(value)
	}

	// 验证超时时间
	if strings.Contains(upper, "TIMEOUT") {
		return v.validatePositiveInt(value)
	}

	// 验证 HTTP 地址
	if upper == "HTTP_ADDR" {
		return v.validateHTTPAddr(value)
	}

	return nil
}

// validatePath 验证路径存在且可访问
func (v *ConfigValidator) validatePath(path string) error {
	// 如果是相对路径，不验证存在性（可能是配置路径）
	if !strings.HasPrefix(path, "/") && !strings.Contains(path, ":") {
		return nil
	}

	// 验证绝对路径存在性
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("path does not exist: %s", path)
	}

	return nil
}

// validateURL 验证 URL 格式
func (v *ConfigValidator) validateURL(urlStr string) error {
	parsedURL, err := url.Parse(urlStr)
	if err != nil {
		return fmt.Errorf("invalid URL format: %w", err)
	}

	if parsedURL.Scheme == "" {
		return fmt.Errorf("URL must have a scheme (http/https)")
	}

	if parsedURL.Host == "" {
		return fmt.Errorf("URL must have a host")
	}

	return nil
}

// validateAPIKey 验证 API Key 不为空
func (v *ConfigValidator) validateAPIKey(key string) error {
	if strings.TrimSpace(key) == "" {
		return fmt.Errorf("API key cannot be empty")
	}
	return nil
}

// validateModel 验证模型名称不为空
func (v *ConfigValidator) validateModel(model string) error {
	if strings.TrimSpace(model) == "" {
		return fmt.Errorf("model name cannot be empty")
	}
	return nil
}

// validatePositiveInt 验证为正整数
func (v *ConfigValidator) validatePositiveInt(value string) error {
	num, err := strconv.Atoi(value)
	if err != nil {
		return fmt.Errorf("must be a valid integer")
	}

	if num <= 0 {
		return fmt.Errorf("must be a positive integer")
	}

	return nil
}

// validateHTTPAddr 验证 HTTP 地址格式
func (v *ConfigValidator) validateHTTPAddr(addr string) error {
	if !strings.HasPrefix(addr, ":") && !strings.Contains(addr, ":") {
		return fmt.Errorf("invalid HTTP address format (expected :port or host:port)")
	}

	// 提取端口号
	parts := strings.Split(addr, ":")
	if len(parts) < 2 {
		return fmt.Errorf("invalid HTTP address format")
	}

	port := parts[len(parts)-1]
	portNum, err := strconv.Atoi(port)
	if err != nil {
		return fmt.Errorf("invalid port number")
	}

	if portNum < 1 || portNum > 65535 {
		return fmt.Errorf("port number must be between 1 and 65535")
	}

	return nil
}

// isRequired 检查字段是否为必填
func (v *ConfigValidator) isRequired(key string) bool {
	upper := strings.ToUpper(key)

	// 必填字段列表
	requiredFields := []string{
		"WORKSPACE_ROOT",
		"CMD_TIMEOUT_SEC",
		"HTTP_ADDR",
		"OPENAI_BASE_URL",
		"OPENAI_API_KEY",
		"OPENAI_MODEL",
		"EXECUTOR_MAX_ITERATIONS",
		"PLAN_EXECUTE_MAX_ITERATIONS",
	}

	for _, required := range requiredFields {
		if strings.Contains(upper, required) {
			return true
		}
	}

	// BUILD_MAX_RETRIES 也是必填
	if upper == "BUILD_MAX_RETRIES" {
		return true
	}

	return false
}
