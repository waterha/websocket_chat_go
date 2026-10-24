package main

import (
	"embed"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/wailsapp/wails/v2"
	"github.com/wailsapp/wails/v2/pkg/options"
	"github.com/wailsapp/wails/v2/pkg/options/assetserver"
	"github.com/wailsapp/wails/v2/pkg/options/windows"
	"github.com/wailsapp/wails/v2/pkg/runtime"
	"websocket_chat_go/desktop/internal/client"
)

//go:embed all:frontend/dist
var assets embed.FS

// Migrations are copied from database/migrations by build-desktop.ps1 before
// the launcher is built; a fresh checkout only carries the .keep placeholder.
//
//go:embed all:migrations
var migrations embed.FS

func migrationFS() (fs.FS, error) {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		return nil, err
	}
	return sub, nil
}

func writableDataDir() (string, error) {
	var candidates []string
	if configDir, err := os.UserConfigDir(); err == nil {
		candidates = append(candidates, filepath.Join(configDir, "WebsocketChat"))
	}
	if executable, err := os.Executable(); err == nil {
		candidates = append(candidates, filepath.Join(filepath.Dir(executable), "WebsocketChat-data"))
	}
	if temp := os.TempDir(); temp != "" {
		candidates = append(candidates, filepath.Join(temp, "WebsocketChat-data"))
	}
	var failures []string
	for _, candidate := range candidates {
		if err := os.MkdirAll(candidate, 0755); err != nil {
			log.Printf("跳过数据目录候选 %s：%v", candidate, err)
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		probe, err := os.CreateTemp(candidate, ".write-test-*")
		if err != nil {
			log.Printf("跳过数据目录候选 %s：%v", candidate, err)
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		probeName := probe.Name()
		if err := probe.Close(); err != nil {
			_ = os.Remove(probeName)
			log.Printf("跳过数据目录候选 %s：%v", candidate, err)
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		if err := os.Remove(probeName); err != nil {
			log.Printf("跳过数据目录候选 %s：%v", candidate, err)
			failures = append(failures, fmt.Sprintf("%s: %v", candidate, err))
			continue
		}
		return candidate, nil
	}
	return "", fmt.Errorf("没有可写的应用数据目录：%s", strings.Join(failures, "; "))
}

func main() {
	root, err := writableDataDir()
	if err != nil {
		log.Fatal(err)
	}
	log.Printf("应用数据目录：%s", root)
	// Keep the WebView2 profile beside the app settings so its cookies and local
	// storage survive restarts. A fresh temp profile is safer than WebView2's
	// default fallback when a previous profile has unusable permissions.
	if err := os.MkdirAll(root, 0755); err != nil {
		log.Printf("无法创建应用数据目录 %q：%v", root, err)
	}
	webviewPath := filepath.Join(root, "WebView2")
	if err := os.MkdirAll(webviewPath, 0755); err != nil {
		fallback, fallbackErr := os.MkdirTemp(root, "WebsocketChat-WebView2-")
		if fallbackErr != nil {
			log.Fatalf("无法创建 WebView2 数据目录 %q：%v；备用目录也不可用：%v", webviewPath, err, fallbackErr)
		}
		log.Printf("应用数据目录不可用，将使用临时 WebView2 数据目录 %q：%v", fallback, err)
		webviewPath = fallback
	}
	path := filepath.Join(root, "settings.json")
	config, err := client.Load(path)
	if err != nil {
		log.Fatal(err)
	}
	embeddedMigrations, err := migrationFS()
	if err != nil {
		log.Fatal(err)
	}
	app, err := newApp(path, root, embeddedMigrations, config)
	if err != nil {
		log.Fatal(err)
	}
	apiMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
			if strings.HasPrefix(request.URL.Path, "/api/v1/") {
				app.serveHTTP(writer, request)
				return
			}
			next.ServeHTTP(writer, request)
		})
	}
	err = wails.Run(&options.App{
		Title: "协作聊天", Width: 430, Height: 460, MinWidth: 430, MinHeight: 460, Frameless: true,
		BackgroundColour: options.NewRGB(247, 242, 234),
		AssetServer:      &assetserver.Options{Assets: assets, Handler: http.HandlerFunc(app.serveHTTP), Middleware: apiMiddleware},
		OnStartup:        app.startup, OnShutdown: app.shutdown, Bind: []interface{}{app},
		SingleInstanceLock: &options.SingleInstanceLock{UniqueId: "5ac5da33-3af2-4e70-a638-f883d41f1026", OnSecondInstanceLaunch: func(options.SecondInstanceData) { runtime.WindowUnminimise(app.ctx); runtime.WindowShow(app.ctx) }},
		Windows:            &windows.Options{WebviewUserDataPath: webviewPath, WebviewGpuIsDisabled: true, Theme: windows.Light},
	})
	if err != nil {
		log.Fatal(err)
	}
}
