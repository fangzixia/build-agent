package main

import (
	"log"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"

	"build-agent/frontend"
	"build-agent/internal/config"
	"build-agent/internal/desktop"
)

func main() {
	// 加载配置
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 创建 Wails Bridge
	bridge := desktop.NewBridge(cfg)

	// 创建应用
	err = wails.Run(&options.App{
		Title:     "Build Agent",
		Width:     1280,
		Height:    800,
		MinWidth:  800,
		MinHeight: 600,
		AssetServer: &assetserver.Options{
			Assets: frontend.FS(),
		},
		OnStartup:  bridge.Startup,
		OnShutdown: bridge.Shutdown,
		Bind: []interface{}{
			bridge,
		},
		BackgroundColour: &options.RGBA{R: 27, G: 38, B: 54, A: 1},
	})

	if err != nil {
		log.Fatalf("Failed to start Wails application: %v", err)
	}
}
