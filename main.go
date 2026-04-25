package main

import (
	"bruv/internal/config"
	"embed"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	// Create an instance of the app structure
	app := NewApp()

	// Load saved window bounds (if any)
	width, height := 1280, 800
	startHidden := false
	if wb := config.LoadWindowBounds(); wb != nil {
		app.savedBounds = wb
		width = wb.Width
		height = wb.Height
		startHidden = true // we'll show after positioning in domReady
	}

	// Create application with options
	err := wails.Run(&options.App{
		Title:       "BRUV",
		Width:       width,
		Height:      height,
		MinWidth:    800,
		MinHeight:   600,
		StartHidden: startHidden,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 24, B: 27, A: 1},
		OnStartup:        app.startup,
		OnDomReady:       app.domReady,
		OnBeforeClose:    app.beforeClose,
		// Only the shell-bridge surface is exposed to the frontend via
		// Wails IPC; the full domain API (~130 methods) is reached over
		// HTTP+SSE through core/services + transport/http. See shell_bridge.go.
		Bind: []interface{}{
			newShellAPI(app),
		},
	})

	if err != nil {
		println("Error:", err.Error())
	}
}
