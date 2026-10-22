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
	"github.com/wailsapp/wails/v2/pkg/options/windows"
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
	// Keep the WebView2 profile beside the app settings so its cookies and local
	// storage survive restarts. A fresh temp profile is safer than WebView2's
	// default fallback when a previous profile has unusable permissions.
	if err := os.MkdirAll(root, 0755); err != nil {
		log.Printf("无法创建应用数据目录 %q：%v", root, err)
	}
	webviewPath := filepath.Join(root, "WebView2")
	if err := os.MkdirAll(webviewPath, 0755); err != nil {
		fallback, fallbackErr := os.MkdirTemp("", "WebsocketChat-WebView2-")
		if fallbackErr != nil {
			log.Fatalf("无法创建 WebView2 数据目录 %q：%v；临时目录也不可用：%v", webviewPath, err, fallbackErr)
		}
		log.Printf("应用数据目录不可用，将使用临时 WebView2 数据目录 %q：%v", fallback, err)
		webviewPath = fallback
	}
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
		Windows:            &windows.Options{WebviewUserDataPath: webviewPath, WebviewGpuIsDisabled: true, Theme: windows.Light},
	})
	if err != nil {
		log.Fatal(err)
	}
}
