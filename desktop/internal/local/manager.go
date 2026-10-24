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

// Manager owns the service stack started for the launcher. Resources stay
// registered until Stop so a failed attempt still leaves its processes
// reachable for cleanup and for a later retry.
type Manager struct {
	root       string
	migrations fs.FS

	mu        sync.Mutex
	resources []*runHandle
}

type runHandle struct {
	cancel context.CancelFunc
	done   chan error
	active bool

	pg    *postgresService
	redis *redisService
}

func New(root string, migrations fs.FS) *Manager {
	return &Manager{root: root, migrations: migrations}
}

// Run brings the local stack up and blocks until the chat server answers on
// the target port. It returns nil immediately when a chat server is already
// running there, whatever process owns it.
func (manager *Manager) Run(ctx context.Context, config client.Config, progress func(string)) error {
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
	handle := &runHandle{}
	manager.mu.Lock()
	manager.resources = append(manager.resources, handle)
	manager.mu.Unlock()

	progress(fmt.Sprintf("正在准备本机数据库（PostgreSQL，端口 %d）", manager.postgresPort()))
	pg, err := manager.ensurePostgres(ctx, progress)
	if err != nil {
		return err
	}
	manager.mu.Lock()
	handle.pg = pg
	manager.mu.Unlock()
	progress(fmt.Sprintf("正在准备 Redis（端口 %d）", manager.redisPort()))
	redis, err := manager.ensureRedis(ctx, progress)
	if err != nil {
		return err
	}
	manager.mu.Lock()
	handle.redis = redis
	manager.mu.Unlock()

	migrationsDir := filepath.Join(manager.root, "migrations")
	progress("正在更新数据库结构")
	if err := extractMigrations(manager.migrations, migrationsDir); err != nil {
		return err
	}
	progress(fmt.Sprintf("正在启动聊天服务（端口 %d）", target.Port))
	runContext, cancel := context.WithCancel(ctx)
	done := make(chan error, 1)
	manager.mu.Lock()
	handle.cancel = cancel
	handle.done = done
	handle.active = true
	manager.mu.Unlock()
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
		defer cancel()
		done <- localsvc.Run(runContext, options)
	}()
	deadline := time.Now().Add(2 * time.Minute)
	for {
		if healthy(ctx, target) {
			return nil
		}
		select {
		case err := <-done:
			if err != nil {
				return fmt.Errorf("聊天服务启动失败：%w", err)
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
	bin := FindPostgres()
	if bin == "" {
		return nil, fmt.Errorf("未找到 PostgreSQL。请安装 PostgreSQL 12+，或设置环境变量 IM_POSTGRES_BIN 指向其 bin 目录")
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
	if err := service.ensure(ctx, progress); err != nil {
		return nil, err
	}
	return service, nil
}

func (manager *Manager) ensureRedis(ctx context.Context, progress func(string)) (*redisService, error) {
	bin := FindRedis(manager.root)
	if bin == "" {
		if os.Getenv("IM_REDIS_DIR") != "" {
			return nil, errors.New("环境变量 IM_REDIS_DIR 指向的目录中没有 redis-server.exe，请修正后重试")
		}
		progress("首次运行：下载便携版 Redis（约 15 MB）")
		if err := downloadRedis(ctx, manager.root); err != nil {
			return nil, err
		}
		bin = FindRedis(manager.root)
		if bin == "" {
			return nil, errors.New("Redis 下载后仍未找到 redis-server.exe，请设置环境变量 IM_REDIS_DIR 指向其目录")
		}
	}
	service := &redisService{bin: bin, data: filepath.Join(manager.root, "redis-data"), port: manager.redisPort()}
	if err := service.ensure(ctx, progress); err != nil {
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
	resources := append([]*runHandle(nil), manager.resources...)
	manager.resources = nil
	handles := make([]*runHandle, 0, len(resources))
	for _, handle := range resources {
		handles = append(handles, &runHandle{cancel: handle.cancel, done: handle.done, active: handle.active, pg: handle.pg, redis: handle.redis})
	}
	manager.mu.Unlock()
	for _, handle := range handles {
		if handle.active && handle.cancel != nil {
			handle.cancel()
			select {
			case <-handle.done:
			case <-time.After(timeout):
			}
		}
		if handle.redis != nil {
			handle.redis.stop()
		}
		if handle.pg != nil {
			handle.pg.stop()
		}
	}
}
