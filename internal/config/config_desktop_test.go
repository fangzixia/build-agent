package config

import (
	"os"
	"testing"
)

func TestDesktopConfig_DefaultValues(t *testing.T) {
	// 清理环境变量
	os.Clearenv()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 验证默认值
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"WindowTitle", cfg.Desktop.WindowTitle, "Build Agent"},
		{"WindowWidth", cfg.Desktop.WindowWidth, 1280},
		{"WindowHeight", cfg.Desktop.WindowHeight, 800},
		{"MinWidth", cfg.Desktop.MinWidth, 800},
		{"MinHeight", cfg.Desktop.MinHeight, 600},
		{"EnableTray", cfg.Desktop.EnableTray, true},
		{"TrayIcon", cfg.Desktop.TrayIcon, ""},
		{"DevMode", cfg.Desktop.DevMode, false},
		{"DevServerURL", cfg.Desktop.DevServerURL, ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestDesktopConfig_CustomValues(t *testing.T) {
	// 设置自定义环境变量
	os.Clearenv()
	os.Setenv("DESKTOP_WINDOW_TITLE", "Custom Title")
	os.Setenv("DESKTOP_WINDOW_WIDTH", "1920")
	os.Setenv("DESKTOP_WINDOW_HEIGHT", "1080")
	os.Setenv("DESKTOP_MIN_WIDTH", "1024")
	os.Setenv("DESKTOP_MIN_HEIGHT", "768")
	os.Setenv("DESKTOP_ENABLE_TRAY", "false")
	os.Setenv("DESKTOP_TRAY_ICON", "custom.ico")
	os.Setenv("DESKTOP_DEV_MODE", "true")
	os.Setenv("DESKTOP_DEV_SERVER_URL", "http://localhost:3000")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	// 验证自定义值
	tests := []struct {
		name     string
		got      interface{}
		expected interface{}
	}{
		{"WindowTitle", cfg.Desktop.WindowTitle, "Custom Title"},
		{"WindowWidth", cfg.Desktop.WindowWidth, 1920},
		{"WindowHeight", cfg.Desktop.WindowHeight, 1080},
		{"MinWidth", cfg.Desktop.MinWidth, 1024},
		{"MinHeight", cfg.Desktop.MinHeight, 768},
		{"EnableTray", cfg.Desktop.EnableTray, false},
		{"TrayIcon", cfg.Desktop.TrayIcon, "custom.ico"},
		{"DevMode", cfg.Desktop.DevMode, true},
		{"DevServerURL", cfg.Desktop.DevServerURL, "http://localhost:3000"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.expected {
				t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.expected)
			}
		})
	}
}

func TestGetEnvBool(t *testing.T) {
	tests := []struct {
		name     string
		envValue string
		fallback bool
		expected bool
	}{
		{"empty string uses fallback", "", true, true},
		{"true string", "true", false, true},
		{"false string", "false", true, false},
		{"1 is true", "1", false, true},
		{"0 is false", "0", true, false},
		{"invalid uses fallback", "invalid", true, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			os.Clearenv()
			if tt.envValue != "" {
				os.Setenv("TEST_BOOL", tt.envValue)
			}
			got := getEnvBool("TEST_BOOL", tt.fallback)
			if got != tt.expected {
				t.Errorf("getEnvBool() = %v, want %v", got, tt.expected)
			}
		})
	}
}
