package main

import (
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsopts "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()

	err := wails.Run(&options.App{
		Title:     "codex-lover",
		Width:     1280,
		Height:    860,
		MinWidth:  980,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 242, G: 246, B: 250, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []any{
			app,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "codex-lover.desktop",
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		Windows: &windowsopts.Options{
			WebviewUserDataPath: "",
			DisablePinchZoom:    true,
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
