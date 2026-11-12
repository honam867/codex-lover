package main

import (
	"embed"
	"os"
	"slices"

	"codex-lover/internal/store"
	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	windowsopts "github.com/wailsapp/wails/v2/pkg/options/windows"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	// Both are bound in every mode so the generated JS bindings cover both.
	widget := &Widget{isWidget: slices.Contains(os.Args[1:], widgetFlag)}

	var err error
	if widget.isWidget {
		err = runWidget(app, widget)
	} else {
		err = runMain(app, widget)
	}
	if err != nil {
		println("Error:", err.Error())
	}
}

func runMain(app *App, widget *Widget) error {
	return wails.Run(&options.App{
		Title:     "codex-lover",
		Width:     1280,
		Height:    860,
		MinWidth:  980,
		MinHeight: 700,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 10, G: 10, B: 15, A: 1},
		OnStartup:        app.startup,
		OnShutdown:       app.shutdown,
		Bind: []any{
			app,
			widget,
		},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId:               "codex-lover.desktop",
			OnSecondInstanceLaunch: app.onSecondInstanceLaunch,
		},
		Windows: &windowsopts.Options{
			WebviewUserDataPath: "",
			DisablePinchZoom:    true,
			WindowIsTranslucent: true,
			BackdropType:        windowsopts.Acrylic,
			Theme:               windowsopts.Dark,
		},
	})
}

func runWidget(app *App, widget *Widget) error {
	st, err := store.New()
	if err != nil {
		return err
	}
	widget.store = st
	return wails.Run(&options.App{
		Title:            "codex-lover quota",
		Width:            300,
		Height:           150,
		DisableResize:    true,
		Frameless:        true,
		AlwaysOnTop:      true,
		AssetServer:      &assetserver.Options{Assets: assets},
		BackgroundColour: &options.RGBA{R: 0, G: 0, B: 0, A: 0},
		OnStartup:        widget.startup,
		Bind:             []any{app, widget},
		SingleInstanceLock: &options.SingleInstanceLock{
			UniqueId: "codex-lover.widget",
		},
		Windows: &windowsopts.Options{
			// Separate WebView2 profile: two processes cannot share one data folder.
			WebviewUserDataPath:  widgetWebviewDataPath(),
			DisablePinchZoom:     true,
			WebviewIsTransparent: true,
			WindowIsTranslucent:  true,
			BackdropType:         windowsopts.Acrylic,
			Theme:                windowsopts.Dark,
		},
	})
}
