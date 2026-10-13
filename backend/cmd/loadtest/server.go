package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"runtime"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/api"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

type verification struct {
	Online        int   `json:"online_connections"`
	Members       int   `json:"members"`
	LastSeq       int64 `json:"last_seq"`
	Confirmed     int   `json:"confirmed_members"`
	PendingOutbox int   `json:"pending_outbox"`
	ReadMembers   int   `json:"read_members"`
	MinReceived   int64 `json:"min_received_seq"`
	MaxReceived   int64 `json:"max_received_seq"`
}

func verify(ctx context.Context, services *svc.ServiceContext, group string) (verification, error) {
	result := verification{Online: len(services.Hub.Clients())}
	err := services.Store.DB.QueryRow(ctx, `SELECT c.next_seq,count(*),count(*) FILTER (WHERE COALESCE(dc.received_seq,0)=c.next_seq),
		COALESCE(min(dc.received_seq),0),COALESCE(max(dc.received_seq),0),count(*) FILTER (WHERE cm.last_read_seq>0),
		(SELECT count(*) FROM message_outbox WHERE conversation_id=$1 AND published_at IS NULL)
		FROM conversations c JOIN conversation_members cm ON cm.conversation_id=c.id
		LEFT JOIN device_cursors dc ON dc.conversation_id=c.id AND dc.user_id=cm.user_id AND dc.device_id='load'
		WHERE c.id=$1 GROUP BY c.id`, group).Scan(&result.LastSeq, &result.Members, &result.Confirmed, &result.MinReceived, &result.MaxReceived, &result.ReadMembers, &result.PendingOutbox)
	return result, err
}

func sample(ctx context.Context, services *svc.ServiceContext, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0600)
	if err != nil {
		return err
	}
	defer file.Close()
	encoder := json.NewEncoder(file)
	timer := time.NewTicker(time.Second)
	defer timer.Stop()
	for {
		select {
		case <-ctx.Done():
			return nil
		case <-timer.C:
			var memory runtime.MemStats
			runtime.ReadMemStats(&memory)
			pool := services.Store.DB.Stat()
			err = encoder.Encode(map[string]any{
				"time": time.Now().UTC(), "online": len(services.Hub.Clients()), "heap_bytes": memory.HeapAlloc,
				"go_sys_bytes": memory.Sys, "goroutines": runtime.NumGoroutine(), "gc_count": memory.NumGC,
				"pool_acquired": pool.AcquiredConns(), "pool_max": pool.MaxConns(),
				"pool_empty_acquire_count": pool.EmptyAcquireCount(), "pool_acquire_duration_ms": float64(pool.AcquireDuration()) / float64(time.Millisecond),
			})
			if err != nil {
				return err
			}
		}
	}
}

func serve(parent context.Context, settings options) error {
	host, _, err := net.SplitHostPort(settings.address)
	if err != nil || net.ParseIP(host) == nil || !net.ParseIP(host).IsLoopback() {
		return fmt.Errorf("test server must bind a loopback IP")
	}
	databaseURL := os.Getenv("IM_TEST_DATABASE_URL")
	redisAddress := os.Getenv("IM_TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddress == "" {
		return fmt.Errorf("set isolated IM_TEST_DATABASE_URL and IM_TEST_REDIS_ADDR")
	}
	ctx, cancel := context.WithTimeout(parent, 20*time.Minute)
	defer cancel()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return err
	}
	defer admin.Close()
	schema := "im_load_" + model.NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+pgx.Identifier{schema}.Sanitize()); err != nil {
		return err
	}
	defer func() {
		cleanup, stop := context.WithTimeout(context.Background(), 15*time.Second)
		defer stop()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+pgx.Identifier{schema}.Sanitize()+" CASCADE"); err != nil {
			log.Printf("test schema cleanup: %v", err)
		}
	}()
	postgres, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		return err
	}
	postgres.ConnConfig.RuntimeParams["search_path"] = schema
	postgres.MaxConns = 16
	data := fixture{URL: "http://" + settings.address, Origin: "http://im.load.test", AdminToken: model.NewID(), Accounts: make([]credentials, settings.members)}
	services, err := svc.New(ctx, config.Config{Postgres: postgres, RedisAddr: redisAddress, Origin: data.Origin, MigrationsDir: settings.migrations, Heartbeat: 20 * time.Second, HeartbeatTimeout: 8 * time.Second})
	if err != nil {
		return err
	}
	defer services.Close()
	defer cancel()
	users, sessions := make([][]any, settings.members), make([][]any, settings.members)
	memberIDs := make([]string, settings.members-1)
	for index := range data.Accounts {
		account := credentials{UserID: model.NewID(), Token: model.NewID() + model.NewID()}
		data.Accounts[index] = account
		users[index] = []any{account.UserID, fmt.Sprintf("load-%04d@example.test", index), fmt.Sprintf("load-%04d", index), "not-a-login-password"}
		sessions[index] = []any{model.TokenHash(account.Token), account.UserID, time.Now().Add(time.Hour)}
		if index > 0 {
			memberIDs[index-1] = account.UserID
		}
	}
	if _, err = services.Store.DB.CopyFrom(ctx, pgx.Identifier{"users"}, []string{"id", "email", "name", "password_hash"}, pgx.CopyFromRows(users)); err != nil {
		return err
	}
	if _, err = services.Store.DB.CopyFrom(ctx, pgx.Identifier{"sessions"}, []string{"token_hash", "user_id", "expires_at"}, pgx.CopyFromRows(sessions)); err != nil {
		return err
	}
	data.Group, err = services.Chat.Create(ctx, data.Accounts[0].UserID, model.CreateInput{Kind: "group", Name: "isolated websocket load test", MemberIDs: memberIDs})
	if err != nil {
		return err
	}
	services.Start(ctx)
	mux := http.NewServeMux()
	mux.Handle("/", (&api.Server{Services: services}).Handler())
	mux.HandleFunc("GET /_loadtest/verify", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Loadtest-Token") != data.AdminToken {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		result, err := verify(request.Context(), services, data.Group)
		if err != nil {
			http.Error(writer, err.Error(), 500)
			return
		}
		writer.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(writer).Encode(result)
	})
	mux.HandleFunc("POST /_loadtest/stop", func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Loadtest-Token") != data.AdminToken {
			http.Error(writer, "forbidden", http.StatusForbidden)
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		log.Print("test server stop requested")
		cancel()
	})
	server := &http.Server{Addr: settings.address, Handler: mux, ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second}
	listener, err := net.Listen("tcp", settings.address)
	if err != nil {
		return err
	}
	defer listener.Close()
	if err = writeJSON(settings.fixture, data); err != nil {
		return err
	}
	defer os.Remove(settings.fixture)
	var workers sync.WaitGroup
	workers.Add(1)
	go func() {
		defer workers.Done()
		<-ctx.Done()
		log.Printf("test server shutdown: %v", ctx.Err())
		services.Hub.Close()
		shutdown, stop := context.WithTimeout(context.Background(), 10*time.Second)
		defer stop()
		_ = server.Shutdown(shutdown)
	}()
	if settings.metrics != "" {
		workers.Add(1)
		go func() {
			defer workers.Done()
			if err := sample(ctx, services, settings.metrics); err != nil {
				log.Print(err)
			}
		}()
	}
	log.Printf("isolated test server ready: %s, %d accounts, pool=16", data.URL, settings.members)
	err = server.Serve(listener)
	log.Printf("test listener stopped: %v", err)
	cancel()
	workers.Wait()
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
