package applog

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"
)

var (
	bridgeLogger *Logger
	aiLogger     *Logger
	once         sync.Once
)

// Logger 是一个简单的按日滚动文件日志
type Logger struct {
	mu     sync.Mutex
	dir    string
	prefix string
	file   *os.File
	day    string
}

func newLogger(dir, prefix string) *Logger {
	_ = os.MkdirAll(dir, 0755)
	return &Logger{dir: dir, prefix: prefix}
}

func (l *Logger) write(level, msg string, fields map[string]any) {
	l.mu.Lock()
	defer l.mu.Unlock()

	today := time.Now().Format("2006-01-02")
	if l.file == nil || l.day != today {
		if l.file != nil {
			l.file.Close()
		}
		path := filepath.Join(l.dir, fmt.Sprintf("%s-%s.log", l.prefix, today))
		f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			return
		}
		l.file = f
		l.day = today
	}

	entry := map[string]any{
		"time":  time.Now().Format(time.RFC3339),
		"level": level,
		"msg":   msg,
	}
	for k, v := range fields {
		entry[k] = v
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	l.file.Write(append(data, '\n'))
}

func (l *Logger) Info(msg string, fields map[string]any)  { l.write("INFO", msg, fields) }
func (l *Logger) Error(msg string, fields map[string]any) { l.write("ERROR", msg, fields) }

// Init 初始化日志，logDir 为日志目录
func Init(logDir string) {
	once.Do(func() {
		bridgeLogger = newLogger(logDir, "bridge")
		aiLogger = newLogger(logDir, "ai")
	})
}

// Bridge 记录接口调用日志
func Bridge(method string, fields map[string]any) {
	if bridgeLogger == nil {
		return
	}
	bridgeLogger.Info(method, fields)
}

// BridgeError 记录接口错误日志
func BridgeError(method string, err error, fields map[string]any) {
	if bridgeLogger == nil {
		return
	}
	if fields == nil {
		fields = map[string]any{}
	}
	fields["error"] = err.Error()
	bridgeLogger.Error(method, fields)
}

// AI 记录 AI 执行日志
func AI(msg string, fields map[string]any) {
	if aiLogger == nil {
		return
	}
	aiLogger.Info(msg, fields)
}
