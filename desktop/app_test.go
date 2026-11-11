package main

import (
	"context"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"websocket_chat_go/desktop/internal/client"
)

type cancelTestManager struct {
	stops atomic.Int64
}

func (m *cancelTestManager) Run(ctx context.Context, _ client.Config, _ func(string)) error {
	<-ctx.Done()
	return ctx.Err()
}

func (m *cancelTestManager) Stop(time.Duration) { m.stops.Add(1) }

func TestBootstrapRegistersCancellationBeforeReturning(t *testing.T) {
	a, err := newApp("", t.TempDir(), fstest.MapFS{}, client.Config{ServerURL: "http://127.0.0.1:8080"})
	if err != nil {
		t.Fatal(err)
	}
	manager := &cancelTestManager{}
	a.manager = manager
	a.beginBootstrap()
	a.bootMu.Lock()
	registered := a.bootCancel != nil
	a.bootMu.Unlock()
	if !registered {
		t.Fatal("bootstrap returned before registering shutdown cancellation")
	}
	a.shutdown(context.Background())
	if got := manager.stops.Load(); got != 1 {
		t.Fatalf("manager Stop calls = %d, want 1", got)
	}
	// A late bridge retry may race with window teardown, but must not schedule
	// new services after shutdown has already cleaned up the manager.
	a.setBootstrap(bootstrapState{Stage: "failed"})
	a.RetryBootstrap()
	if state := a.BootstrapStatus(); state.Stage != "failed" {
		t.Fatalf("retry after shutdown changed bootstrap state: %+v", state)
	}
}
