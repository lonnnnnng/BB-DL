package main

import (
	"embed"
	"fmt"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/mac"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	app := NewApp()
	err := wails.Run(&options.App{
		Title:                    "BB-DL",
		Width:                    1440,
		Height:                   900,
		MinWidth:                 1024,
		MinHeight:                680,
		Frameless:                false,
		StartHidden:              false,
		BackgroundColour:         &options.RGBA{R: 244, G: 246, B: 248, A: 1},
		AssetServer:              &assetserver.Options{Assets: assets},
		OnStartup:                app.startup,
		OnShutdown:               app.shutdown,
		OnBeforeClose:            app.beforeClose,
		EnableDefaultContextMenu: false,
		Mac: &mac.Options{
			TitleBar: mac.TitleBarHiddenInset(),
			About: &mac.AboutInfo{
				Title:   "BB-DL",
				Message: fmt.Sprintf("版本 %s\n构建时间 %s", appVersion(), appBuildTime()),
			},
		},
		Bind: []interface{}{app},
	})
	if err != nil {
		fmt.Println("启动 BB-DL 失败：", err)
	}
}
