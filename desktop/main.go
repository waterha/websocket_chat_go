package main

import (
	"embed"
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"websocket_chat_go/desktop/internal/client"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	root, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	root = filepath.Join(root, "WebsocketChat")
	path := filepath.Join(root, "settings.json")
	config, err := client.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	app, err := newApp(path, config)
	if err != nil {
		log.Fatal(err)
	}
	err = wails.Run(&options.App{
		Title: "协作聊天", Width: 1280, Height: 820, MinWidth: 900, MinHeight: 600,
		BackgroundColour: options.NewRGB(247, 242, 234),
		AssetServer:      &assetserver.Options{Assets: assets, Handler: http.HandlerFunc(app.serveHTTP)},
		OnStartup:        app.startup, OnShutdown: app.shutdown, Bind: []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "5ac5da33-3af2-4e70-a638-f883d41f1026", OnSecondInstanceLaunch: func(options.SecondInstanceData) { runtime.WindowUnminimise(app.ctx); runtime.WindowShow(app.ctx) }},
	})
	if err != nil {
		log.Fatal(err)
	}
}
