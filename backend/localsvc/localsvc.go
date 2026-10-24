// Package localsvc runs the chat server inside another process, such as the
// desktop launcher. It mirrors cmd/server but takes explicit settings instead
// of reading .env files, so the host controls every value.
package localsvc

import (
	"context"
	"errors"
	"log"
	"net/http"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/api"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/svc"
)

type Options struct {
	HTTPAddr         string
	Origin           string
	PostgresHost     string
	PostgresPort     uint16
	PostgresUser     string
	PostgresDatabase string
	PostgresPassword string
	RedisAddr        string
	MigrationsDir    string
	MediaDir         string
}

func (options Options) settings() (config.Config, error) {
	postgres, err := pgxpool.ParseConfig("")
	if err != nil {
		return config.Config{}, err
	}
	postgres.ConnConfig.Host = options.PostgresHost
	postgres.ConnConfig.Port = options.PostgresPort
	postgres.ConnConfig.User = options.PostgresUser
	postgres.ConnConfig.Database = options.PostgresDatabase
	postgres.ConnConfig.Password = options.PostgresPassword
	postgres.ConnConfig.ConnectTimeout = 5 * time.Second
	postgres.MaxConns = 16
	// The local cluster never speaks TLS, and libpq environment variables from
	// the host machine must not override that.
	postgres.ConnConfig.TLSConfig = nil
	return config.Config{
		HTTPAddr: options.HTTPAddr, Origin: options.Origin,
		Postgres: postgres, RedisAddr: options.RedisAddr,
		MigrationsDir: options.MigrationsDir, MediaDir: options.MediaDir,
		Heartbeat: 20 * time.Second, HeartbeatTimeout: 8 * time.Second,
	}, nil
}

// ParsePort converts a port number from the host application, falling back to
// the backend default when it is missing or out of range.
func ParsePort(port string) uint16 {
	value, err := strconv.ParseUint(port, 10, 16)
	if err != nil || value == 0 {
		return 8080
	}
	return uint16(value)
}

// Run serves the chat API until ctx is cancelled. It returns nil after a clean
// shutdown, matching the behaviour of the standalone server command.
func Run(ctx context.Context, options Options) error {
	settings, err := options.settings()
	if err != nil {
		return err
	}
	startup, cancel := context.WithTimeout(ctx, 30*time.Second)
	services, err := svc.New(startup, settings)
	cancel()
	if err != nil {
		return err
	}
	workerContext, stopWorkers := context.WithCancel(ctx)
	services.Start(workerContext)
	defer services.Close()
	defer stopWorkers()
	server := &http.Server{Addr: options.HTTPAddr, Handler: (&api.Server{Services: services}).Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		services.Hub.Close()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("localsvc shutdown: %v", err)
		}
	}()
	err = server.ListenAndServe()
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
