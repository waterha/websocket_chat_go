package svc

import (
	"context"
	"os"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

type ServiceContext struct {
	Config    config.Config
	Store     *model.Store
	Redis     *redis.Client
	Auth      *logic.Auth
	Chat      *logic.Chat
	Media     *logic.MediaService
	Presence  *logic.Presence
	Hub       *realtime.Hub
	Broadcast *realtime.Broadcast
	workers   sync.WaitGroup
}

func New(ctx context.Context, settings config.Config) (*ServiceContext, error) {
	db, err := pgxpool.NewWithConfig(ctx, settings.Postgres)
	if err != nil {
		return nil, err
	}
	redisClient := redis.NewClient(&redis.Options{Addr: settings.RedisAddr, Password: settings.RedisPassword, DialTimeout: settings.HeartbeatTimeout, ReadTimeout: settings.HeartbeatTimeout, WriteTimeout: settings.HeartbeatTimeout})
	// A subscribed go-redis client dedicates its connection to the subscription,
	// so fan-out uses a second client to keep normal commands working. The read
	// timeout matters: without it ReceiveMessage blocks forever and shutdown
	// could not drain the subscription goroutine.
	subscriber := redis.NewClient(&redis.Options{Addr: settings.RedisAddr, Password: settings.RedisPassword, DialTimeout: settings.HeartbeatTimeout, ReadTimeout: 5 * time.Second, WriteTimeout: settings.HeartbeatTimeout})
	cleanup := func() { db.Close(); _ = redisClient.Close(); _ = subscriber.Close() }
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
	hub := realtime.NewHub(128)
	// Each process needs its own identity so a subscriber can skip the events it
	// published itself and avoid delivering them twice.
	instanceID := os.Getenv("INSTANCE_ID")
	if instanceID == "" {
		instanceID = model.NewID()
	}
	return &ServiceContext{
		Config: settings, Store: store, Redis: redisClient,
		Auth:     &logic.Auth{Store: store, Limiter: &logic.Limiter{Redis: redisClient}},
		Chat:     &logic.Chat{Store: store},
		Media:    &logic.MediaService{Store: store, Files: files},
		Presence: &logic.Presence{Redis: redisClient},
		Hub:      hub,
		Broadcast: &realtime.Broadcast{Publisher: redisClient, Subscriber: subscriber, Hub: hub,
			InstanceID: instanceID, Channel: realtime.DefaultBroadcastChannel},
	}, nil
}

func (services *ServiceContext) Start(ctx context.Context) {
	services.workers.Add(4)
	go func() {
		defer services.workers.Done()
		(&realtime.Dispatcher{Store: services.Store, Hub: services.Hub, Broadcast: services.Broadcast}).Run(ctx)
	}()
	go func() {
		defer services.workers.Done()
		services.Broadcast.Run(ctx)
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
	// Close the subscription client first so a blocked ReceiveMessage returns
	// immediately; otherwise waiting for the workers would hang.
	if services.Broadcast != nil && services.Broadcast.Subscriber != nil {
		_ = services.Broadcast.Subscriber.Close()
	}
	services.workers.Wait()
	services.Store.DB.Close()
	_ = services.Redis.Close()
}
