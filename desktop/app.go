package main

import (
	"context"
	"net/http"
	"path/filepath"
	"strings"
	"sync"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"websocket_chat_go/desktop/internal/client"
)

type App struct {
	ctx    context.Context
	path   string
	mu     sync.RWMutex
	client *client.Client
}

func newApp(path string, config client.Config) (*App, error) {
	c, err := client.New(config)
	if err != nil {
		return nil, err
	}
	return &App{path: path, client: c}, nil
}

func (a *App) current() *client.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

func (a *App) startup(ctx context.Context) { a.ctx = ctx }
func (a *App) shutdown(context.Context)    { a.current().Close() }
func (a *App) Config() client.Config       { return a.current().Config }

func (a *App) SaveConfig(config client.Config) error {
	next, err := client.New(config)
	if err != nil {
		return err
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if err := client.Save(a.path, next.Config); err != nil {
		next.Close()
		return err
	}
	previous := a.client
	a.client = next
	previous.Close()
	return nil
}

func (a *App) CheckServer(config client.Config) error {
	c, err := client.New(config)
	if err != nil {
		return err
	}
	defer c.Close()
	return c.Check(a.ctx)
}

func (a *App) OpenSocket(id, device string) error {
	return a.current().OpenSocket(id, device, func(event client.SocketEvent) {
		runtime.EventsEmit(a.ctx, "desktop:socket", event)
	})
}
func (a *App) SendSocket(id, payload string) error { return a.current().SendSocket(id, payload) }
func (a *App) CloseSocket(id string)               { a.current().CloseSocket(id) }

func (a *App) SaveAttachment(id, name string) (string, error) {
	// The suggested filename is untrusted message content, never a path.
	name = filepath.Base(strings.ReplaceAll(name, "\\", "/"))
	if name == "." || name == "/" || name == "" {
		name = "attachment"
	}
	path, err := runtime.SaveFileDialog(a.ctx, runtime.SaveDialogOptions{Title: "保存附件", DefaultFilename: name})
	if err != nil || path == "" {
		return "", err
	}
	if err := a.current().Download(a.ctx, id, path); err != nil {
		return "", err
	}
	return path, nil
}

func (a *App) serveHTTP(w http.ResponseWriter, r *http.Request) {
	if !strings.HasPrefix(r.URL.Path, "/api/v1/") {
		http.NotFound(w, r)
		return
	}
	a.current().ServeHTTP(w, r)
}
