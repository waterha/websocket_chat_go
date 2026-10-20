package svc

import (
	"context"
	"sync"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

type ServiceContext struct {
	Config  config.Config
	Store   *model.Store
	Redis   *redis.Client
	Auth    *logic.Auth
	Chat    *logic.Chat
	Media   *logic.MediaService
	Presence *logic.Presence
	Hub     *realtime.Hub
	workers sync.WaitGroup
}

func New(ctx context.Context, settings config.Config) (*ServiceContext, error) {
	db, err := pgxpool.NewWithConfig(ctx, settings.Postgres)
	if err != nil {
		return nil, err
	}
	redisClient := redis.NewClient(&redis.Options{Addr: settings.RedisAddr, Password: settings.RedisPassword, DialTimeout: settings.HeartbeatTimeout, ReadTimeout: settings.HeartbeatTimeout, WriteTimeout: settings.HeartbeatTimeout})
	cleanup := func() { db.Close(); _ = redisClient.Close() }
	if err = db.Ping(ctx); err != nil {
		cleanup()
		return nil, err
	}
	if err = redisClient.Ping(ctx).Err(); err != nil {
		cleanup()
		return nil, err
	}
	store := &model.Store{DB: db}
	if err = store.Migrate(ctx, settings.MigrationsDir); err != nil {
		cleanup()
		return nil, err
	}
	files, err := media.NewStore(settings.MediaDir)
	if err != nil {
		cleanup()
		return nil, err
	}
	return &ServiceContext{Config: settings, Store: store, Redis: redisClient, Auth: &logic.Auth{Store: store, Limiter: &logic.Limiter{Redis: redisClient}}, Chat: &logic.Chat{Store: store}, Media: &logic.MediaService{Store: store, Files: files}, Presence: &logic.Presence{Redis: redisClient}, Hub: realtime.NewHub(128)}, nil
}

func (services *ServiceContext) Start(ctx context.Context) {
	services.workers.Add(3)
	go func() {
		defer services.workers.Done()
		(&realtime.Dispatcher{Store: services.Store, Hub: services.Hub}).Run(ctx)
	}()
	go func() {
		defer services.workers.Done()
		(&realtime.Reconciler{Store: services.Store, Hub: services.Hub}).Run(ctx)
	}()
	go func() {
		defer services.workers.Done()
		(&realtime.Retention{Store: services.Store, Files: services.Media.Files}).Run(ctx)
	}()
}

func (services *ServiceContext) Close() {
	services.Hub.Close()
	services.workers.Wait()
	services.Store.DB.Close()
	_ = services.Redis.Close()
}
