package httpserver

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"build-agent/internal/config"
)

// EnvConfigHandler 处理环境配置相关的 HTTP 请求
type EnvConfigHandler struct {
	workspaceRoot string
	envPath       string
	examplePath   string
	parser        *config.EnvParser
	validator     *config.ConfigValidator
}

// NewEnvConfigHandler 创建新的环境配置处理器
func NewEnvConfigHandler(workspaceRoot string) *EnvConfigHandler {
	// .env 文件应该在当前工作目录，而不是 workspaceRoot
	cwd, err := os.Getwd()
	if err != nil {
		cwd = workspaceRoot
	}
	envPath := filepath.Join(cwd, ".env")
	examplePath := filepath.Join(cwd, ".env.example")

	return &EnvConfigHandler{
		workspaceRoot: workspaceRoot,
		envPath:       envPath,
		examplePath:   examplePath,
		parser:        config.NewEnvParser(),
		validator:     config.NewConfigValidator(workspaceRoot),
	}
}

// HandleGetConfig 处理 GET /v1/config/env 请求
func (h *EnvConfigHandler) HandleGetConfig(w http.ResponseWriter, r *http.Request) {
	// 读取 .env 文件
	content, err := os.ReadFile(h.envPath)
	if err != nil {
		if os.IsNotExist(err) {
			// 文件不存在，返回 .env.example 的内容作为模板
			exampleContent, exampleErr := os.ReadFile(h.examplePath)
			if exampleErr == nil {
				// 使用 .env.example 的内容
				envConfig, parseErr := h.parser.Parse(string(exampleContent))
				if parseErr != nil {
					h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse .env.example: %v", parseErr), "PARSE_FAILED", nil)
					return
				}
				writeJSON(w, http.StatusOK, envConfig)
				return
			}
			// .env.example 也不存在，返回空配置
			emptyConfig := &config.EnvConfig{
				Sections: make(map[string][]config.EnvEntry),
			}
			writeJSON(w, http.StatusOK, emptyConfig)
			return
		}
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read .env file: %v", err), "READ_FAILED", nil)
		return
	}

	// 解析配置
	envConfig, err := h.parser.Parse(string(content))
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse .env file: %v", err), "PARSE_FAILED", nil)
		return
	}

	// 返回配置
	writeJSON(w, http.StatusOK, envConfig)
}

// HandleGetExampleConfig 处理 GET /v1/config/env/example 请求
func (h *EnvConfigHandler) HandleGetExampleConfig(w http.ResponseWriter, r *http.Request) {
	// 读取 .env.example 文件
	content, err := os.ReadFile(h.examplePath)
	if err != nil {
		if os.IsNotExist(err) {
			h.writeError(w, http.StatusNotFound, ".env.example file not found", "FILE_NOT_FOUND", nil)
			return
		}
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to read .env.example file: %v", err), "READ_FAILED", nil)
		return
	}

	// 解析配置
	envConfig, err := h.parser.Parse(string(content))
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to parse .env.example file: %v", err), "PARSE_FAILED", nil)
		return
	}

	// 返回配置
	writeJSON(w, http.StatusOK, envConfig)
}

// HandleSaveConfig 处理 POST /v1/config/env 请求
func (h *EnvConfigHandler) HandleSaveConfig(w http.ResponseWriter, r *http.Request) {
	// 解析请求体
	var envConfig config.EnvConfig
	if err := json.NewDecoder(r.Body).Decode(&envConfig); err != nil {
		h.writeError(w, http.StatusBadRequest, "invalid json body", "INVALID_JSON", nil)
		return
	}

	// 验证配置
	validationErrors := h.validator.Validate(&envConfig)
	if len(validationErrors) > 0 {
		h.writeError(w, http.StatusBadRequest, "validation failed", "VALIDATION_FAILED", validationErrors)
		return
	}

	// 序列化配置
	content, err := h.parser.Serialize(&envConfig)
	if err != nil {
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to serialize config: %v", err), "SERIALIZE_FAILED", nil)
		return
	}

	// 原子性写入文件
	if err := h.atomicWriteEnv(content); err != nil {
		h.writeError(w, http.StatusInternalServerError, fmt.Sprintf("failed to write .env file: %v", err), "WRITE_FAILED", nil)
		return
	}

	// 返回成功响应
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"success": true,
		"message": "Configuration saved successfully",
	})
}

// atomicWriteEnv 原子性写入 .env 文件
func (h *EnvConfigHandler) atomicWriteEnv(content string) error {
	// 先写入临时文件
	tmpPath := h.envPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("failed to write temp file: %w", err)
	}

	// 验证写入成功后重命名
	if err := os.Rename(tmpPath, h.envPath); err != nil {
		// 清理临时文件
		os.Remove(tmpPath)
		return fmt.Errorf("failed to rename temp file: %w", err)
	}

	return nil
}

// writeError 写入错误响应
func (h *EnvConfigHandler) writeError(w http.ResponseWriter, statusCode int, message string, code string, details []config.ValidationError) {
	response := map[string]interface{}{
		"error": message,
		"code":  code,
	}

	if details != nil && len(details) > 0 {
		response["details"] = details
	}

	writeJSON(w, statusCode, response)
}
