package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"websocket_chat_go/backend/internal/api"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/svc"
)

func main() {
	if err := run(); err != nil {
		log.Fatal(err)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	settings, err := config.Load()
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
	server := &http.Server{Addr: settings.HTTPAddr, Handler: (&api.Server{Services: services}).Handler(), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	shutdownDone := make(chan struct{})
	go func() {
		defer close(shutdownDone)
		<-ctx.Done()
		services.Hub.Close()
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Println("websocket_chat_go listening on", server.Addr)
	err = server.ListenAndServe()
	stop()
	<-shutdownDone
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}
