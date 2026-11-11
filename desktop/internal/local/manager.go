package local

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sync"
	"time"

	"websocket_chat_go/backend/localsvc"
	"websocket_chat_go/desktop/internal/client"
)

const (
	defaultPostgresPort = 5434
	defaultRedisPort    = 6380
)

// Manager owns the service stack started for the launcher. Startup attempts are
// serialized; Stop permanently closes the gate and cancels registered attempts.
// The startup goroutine retains ownership until it acknowledges cancellation,
// so resources returned after Stop's snapshot are still cleaned up.
type Manager struct {
	root       string
	migrations fs.FS

	startMu   sync.Mutex
	mu        sync.Mutex
	stopped   bool
	resources []*runHandle
}

type runHandle struct {
	cancel      context.CancelFunc
	startupDone chan struct{}
	serverDone  chan struct{}
	serverError error // written before serverDone closes
	cleanupOnce sync.Once
	pg          *postgresService
	redis       *redisService
}

func (handle *runHandle) cleanup(timeout time.Duration) {
	handle.cleanupOnce.Do(func() {
		handle.cancel()
		if handle.serverDone != nil {
			select {
			case <-handle.serverDone:
			case <-time.After(timeout):
			}
		}
		if handle.redis != nil {
			handle.redis.stop()
		}
		if handle.pg != nil {
			handle.pg.stop()
		}
	})
}

func (manager *Manager) beginRun(ctx context.Context) (context.Context, *runHandle, error) {
	manager.mu.Lock()
	defer manager.mu.Unlock()
	if manager.stopped {
		return nil, nil, errors.New("本机服务管理器已关闭")
	}
	if err := ctx.Err(); err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	handle := &runHandle{cancel: cancel, startupDone: make(chan struct{})}
	manager.resources = append(manager.resources, handle)
	return ctx, handle, nil
}

func (manager *Manager) finishRun(handle *runHandle, failed bool) {
	manager.mu.Lock()
	cleanup := failed || manager.stopped
	manager.mu.Unlock()
	if cleanup {
		handle.cleanup(10 * time.Second)
	}
	close(handle.startupDone)
	manager.mu.Lock()
	if cleanup {
		for i, entry := range manager.resources {
			if entry == handle {
				manager.resources = append(manager.resources[:i], manager.resources[i+1:]...)
				break
			}
		}
	}
	manager.mu.Unlock()
}

func New(root string, migrations fs.FS) *Manager {
	return &Manager{root: root, migrations: migrations}
}

// Run brings the local stack up and blocks until the chat server answers on
// the target port. It returns nil immediately when a chat server is already
// running there, whatever process owns it.
func (manager *Manager) Run(ctx context.Context, config client.Config, progress func(string)) (result error) {
	manager.startMu.Lock()
	defer manager.startMu.Unlock()
	ctx, handle, err := manager.beginRun(ctx)
	if err != nil {
		return err
	}
	defer func() { manager.finishRun(handle, result != nil) }()
	if progress == nil {
		progress = func(string) {}
	}
	target, ok := Plan(config)
	if !ok {
		return fmt.Errorf("服务器地址 %s 不是本机 HTTP 地址，启动器不会为它启动服务", config.ServerURL)
	}
	if healthy(ctx, target) {
		progress("本机聊天服务已在运行，直接连接")
		return nil
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	progress(fmt.Sprintf("正在准备本机数据库（PostgreSQL，端口 %d）", manager.postgresPort()))
	pg, err := manager.ensurePostgres(ctx, progress)
	handle.pg = pg
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	progress(fmt.Sprintf("正在准备 Redis（端口 %d）", manager.redisPort()))
	redis, err := manager.ensureRedis(ctx, progress)
	handle.redis = redis
	if err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}

	migrationsDir := filepath.Join(manager.root, "migrations")
	progress("正在更新数据库结构")
	if err := extractMigrations(manager.migrations, migrationsDir); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	progress(fmt.Sprintf("正在启动聊天服务（端口 %d）", target.Port))
	handle.serverDone = make(chan struct{})
	options := localsvc.Options{
		HTTPAddr:         target.Address(),
		Origin:           config.Origin,
		PostgresHost:     "127.0.0.1",
		PostgresPort:     uint16(manager.postgresPort()),
		PostgresUser:     appDBUser,
		PostgresDatabase: appDBName,
		PostgresPassword: pg.appPassword,
		RedisAddr:        redis.address(),
		MigrationsDir:    migrationsDir,
		MediaDir:         mediaDirectory(manager.root),
	}
	go func() {
		defer close(handle.serverDone)
		if err := ctx.Err(); err != nil {
			handle.serverError = err
			return
		}
		handle.serverError = localsvc.Run(ctx, options)
	}()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if healthy(ctx, target) {
			return nil
		}
		select {
		case <-handle.serverDone:
			if handle.serverError != nil {
				return fmt.Errorf("聊天服务启动失败：%w", handle.serverError)
			}
			return fmt.Errorf("聊天服务已退出但端口 %d 未就绪", target.Port)
		case <-ctx.Done():
			return fmt.Errorf("等待聊天服务启动时被取消")
		case <-time.After(500 * time.Millisecond):
		}
		if time.Now().After(deadline) {
			return fmt.Errorf("聊天服务未能在预期时间内就绪（端口 %d）", target.Port)
		}
	}
}

func parsePort(value string) (int, error) {
	port := 0
	if _, err := fmt.Sscanf(value, "%d", &port); err != nil || port <= 0 || port > 65535 {
		return 0, fmt.Errorf("端口 %q 无效", value)
	}
	return port, nil
}

func (manager *Manager) postgresPort() int {
	return envPort("IM_DESKTOP_PGPORT", defaultPostgresPort)
}

func (manager *Manager) redisPort() int {
	return envPort("IM_DESKTOP_REDIS_PORT", defaultRedisPort)
}

func envPort(name string, fallback int) int {
	if value := os.Getenv(name); value != "" {
		if parsed, err := parsePort(value); err == nil {
			return parsed
		}
	}
	return fallback
}

func (manager *Manager) ensurePostgres(ctx context.Context, progress func(string)) (*postgresService, error) {
	bin, err := ensurePostgresBin(ctx, manager.root, progress)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	superPassword, err := loadOrCreatePassword(filepath.Join(manager.root, "pgpass.txt"))
	if err != nil {
		return nil, err
	}
	appPassword, err := loadOrCreatePassword(filepath.Join(manager.root, "db-password.txt"))
	if err != nil {
		return nil, err
	}
	service := &postgresService{
		bin: bin, data: filepath.Join(manager.root, "pgdata"), logs: manager.logDir(),
		port: manager.postgresPort(), superPassword: superPassword, appPassword: appPassword,
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := service.ensure(ctx, progress); err != nil {
		service.stop()
		return nil, err
	}
	return service, nil
}

func (manager *Manager) ensureRedis(ctx context.Context, progress func(string)) (*redisService, error) {
	bin, err := ensureRedisBin(ctx, manager.root, progress)
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	service := &redisService{bin: bin, data: filepath.Join(manager.root, "redis-data"), port: manager.redisPort()}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := service.ensure(ctx, progress); err != nil {
		service.stop()
		return nil, err
	}
	return service, nil
}

func (manager *Manager) logDir() string {
	path := filepath.Join(manager.root, "logs")
	_ = os.MkdirAll(path, 0755)
	return path
}

// Stop shuts the chat server down and then every service the launcher itself
// started. Reused external services keep running, like the dev scripts leave
// them behind.
func (manager *Manager) Stop(timeout time.Duration) {
	manager.mu.Lock()
	manager.stopped = true
	resources := append([]*runHandle(nil), manager.resources...)
	for _, handle := range resources {
		handle.cancel()
	}
	manager.mu.Unlock()
	deadline := time.NewTimer(timeout)
	defer deadline.Stop()
	for _, handle := range resources {
		select {
		case <-handle.startupDone:
			// Startup has published all resources. A canceled in-flight Run
			// cleans itself; successful earlier Runs are cleaned here.
			handle.cleanup(timeout)
		case <-deadline.C:
			// Startup retains ownership and cleans late acquisitions itself.
			return
		}
	}
	manager.mu.Lock()
	manager.resources = nil
	manager.mu.Unlock()
}
