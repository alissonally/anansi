package main

import (
	"context"
	"embed"
	"log/slog"
	"os"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/linux"

	"anansi/internal/api"
	"anansi/internal/security"
)

//go:embed all:frontend/dist
var assets embed.FS

//go:embed build/appicon.png
var icon []byte

func main() {
	// Logs go to stderr and never include SQL text, DSNs or passwords.
	log := slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo}))

	// Secrets saved under the previous name (dbstudio) move to "anansi".
	app, err := api.New(log, security.NewKeyring("anansi", "dbstudio"))
	if err != nil {
		log.Error("startup failed", "error", err.Error())
		os.Exit(1)
	}

	err = wails.Run(&options.App{
		Title:     "Anansi",
		Width:     1360,
		Height:    860,
		MinWidth:  820,
		MinHeight: 520,
		AssetServer: &assetserver.Options{
			Assets: assets,
		},
		BackgroundColour: &options.RGBA{R: 24, G: 26, B: 31, A: 1},
		OnStartup:        app.Startup,
		OnShutdown:       app.Shutdown,
		OnBeforeClose:    func(ctx context.Context) bool { return false },
		Bind:             []interface{}{app},
		Linux: &linux.Options{
			Icon:                icon,
			ProgramName:         "anansi",
			WebviewGpuPolicy:    linux.WebviewGpuPolicyOnDemand,
			WindowIsTranslucent: false,
		},
	})
	if err != nil {
		log.Error("wails", "error", err.Error())
		os.Exit(1)
	}
}
