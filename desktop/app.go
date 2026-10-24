package main

import (
	"context"
	"io/fs"
	"net/http"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v2/pkg/runtime"
	"websocket_chat_go/desktop/internal/client"
	"websocket_chat_go/desktop/internal/local"
)

// bootstrapState is reported to the chat UI while the launcher brings the
// local service stack up. Stages: idle (nothing to manage), running, ready,
// failed.
type bootstrapState struct {
	Stage   string `json:"stage"`
	Message string `json:"message"`
}

type App struct {
	ctx    context.Context
	path   string
	root   string
	mu     sync.RWMutex
	client *client.Client

	bootMu     sync.Mutex
	boot       bootstrapState
	bootCancel context.CancelFunc
	manager    *local.Manager
}

func newApp(path, root string, migrations fs.FS, config client.Config) (*App, error) {
	c, err := client.New(config)
	if err != nil {
		return nil, err
	}
	return &App{path: path, root: root, client: c, manager: local.New(root, migrations)}, nil
}

func (a *App) current() *client.Client {
	a.mu.RLock()
	defer a.mu.RUnlock()
	return a.client
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
	a.beginBootstrap()
}

func (a *App) shutdown(context.Context) {
	a.bootMu.Lock()
	cancel := a.bootCancel
	a.bootMu.Unlock()
	if cancel != nil {
		cancel()
	}
	a.manager.Stop(10 * time.Second)
	a.current().Close()
}
func (a *App) Config() client.Config { return a.current().Config }

func (a *App) SaveConfig(config client.Config) error {
	next, err := client.New(config)
	if err != nil {
		return err
	}
	a.mu.Lock()
	if err := client.Save(a.path, next.Config); err != nil {
		a.mu.Unlock()
		next.Close()
		return err
	}
	previous := a.client
	a.client = next
	a.mu.Unlock()
	previous.Close()
	// A saved local address may need the service stack behind it.
	a.beginBootstrap()
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

// BootstrapStatus lets the chat UI ask for the current launch progress, for
// example right after the window loads.
func (a *App) BootstrapStatus() bootstrapState {
	a.bootMu.Lock()
	defer a.bootMu.Unlock()
	return a.boot
}

// RetryBootstrap restarts the local service stack after a failed attempt.
func (a *App) RetryBootstrap() { a.beginBootstrap() }

// beginBootstrap starts the local stack in the background unless another
// attempt is already running or the configured server is not local.
func (a *App) beginBootstrap() {
	a.bootMu.Lock()
	if a.boot.Stage == "running" {
		a.bootMu.Unlock()
		return
	}
	config := a.current().Config
	if _, ok := local.Plan(config); !ok {
		a.boot = bootstrapState{Stage: "idle"}
		a.bootMu.Unlock()
		return
	}
	a.boot = bootstrapState{Stage: "running", Message: "正在准备本机聊天服务…"}
	a.bootMu.Unlock()
	a.emitBootstrap()
	go a.runBootstrap(config)
}

func (a *App) runBootstrap(config client.Config) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	a.bootMu.Lock()
	if a.bootCancel != nil {
		a.bootCancel()
	}
	a.bootCancel = cancel
	a.bootMu.Unlock()
	err := a.manager.Run(ctx, config, func(message string) {
		a.setBootstrap(bootstrapState{Stage: "running", Message: message})
	})
	if ctx.Err() != nil {
		return
	}
	if err != nil {
		a.setBootstrap(bootstrapState{Stage: "failed", Message: err.Error()})
	} else {
		a.setBootstrap(bootstrapState{Stage: "ready", Message: "本机服务已就绪"})
	}
	a.emitBootstrap()
}

func (a *App) setBootstrap(state bootstrapState) {
	a.bootMu.Lock()
	a.boot = state
	a.bootMu.Unlock()
}

func (a *App) emitBootstrap() {
	a.bootMu.Lock()
	state := a.boot
	a.bootMu.Unlock()
	if a.ctx != nil {
		runtime.EventsEmit(a.ctx, "desktop:bootstrap", state)
	}
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
