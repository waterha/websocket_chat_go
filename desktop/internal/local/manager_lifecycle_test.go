package local

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"websocket_chat_go/desktop/internal/client"
)

func TestManagerStopClosesStartupGate(t *testing.T) {
	manager := New(t.TempDir(), nil)
	manager.Stop(time.Second)
	if err := manager.Run(context.Background(), client.Config{ServerURL: "http://127.0.0.1:8080"}, nil); err == nil {
		t.Fatal("Run after Stop must not start services")
	}
	if len(manager.resources) != 0 {
		t.Fatal("closed manager registered resources")
	}
}

func TestManagerFailedAttemptCleansBeforeRetry(t *testing.T) {
	manager := New(t.TempDir(), nil)
	ctx, handle, err := manager.beginRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	manager.finishRun(handle, true)
	if !errors.Is(ctx.Err(), context.Canceled) {
		t.Fatal("failed attempt kept context alive")
	}
	if len(manager.resources) != 0 {
		t.Fatal("failed attempt retained resources")
	}
	nextCtx, next, err := manager.beginRun(context.Background())
	if err != nil {
		t.Fatalf("retry: %v", err)
	}
	if nextCtx.Err() != nil {
		t.Fatal("retry inherited failed cancellation")
	}
	manager.finishRun(next, true)
}

func TestManagerStopWaitsForLateResourceHandoff(t *testing.T) {
	manager := New(t.TempDir(), nil)
	ctx, handle, err := manager.beginRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	stopped := make(chan struct{})
	go func() { manager.Stop(time.Second); close(stopped) }()
	select {
	case <-ctx.Done():
	case <-time.After(time.Second):
		t.Fatal("Stop did not cancel startup")
	}
	select {
	case <-stopped:
		t.Fatal("Stop returned before startup handed off resources")
	default:
	}
	// Simulate an acquisition that returns after the cancellation snapshot.
	// Stop must not race this write or lose the late server registration.
	handle.serverDone = make(chan struct{})
	close(handle.serverDone)
	manager.finishRun(handle, true)
	select {
	case <-stopped:
	case <-time.After(time.Second):
		t.Fatal("Stop did not finish after handoff")
	}
	if len(manager.resources) != 0 {
		t.Fatal("late resource retained")
	}
}

func TestManagerStopTimeoutLeavesCleanupWithStartupOwner(t *testing.T) {
	manager := New(t.TempDir(), nil)
	ctx, handle, err := manager.beginRun(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	manager.Stop(time.Millisecond)
	if ctx.Err() == nil {
		t.Fatal("timed out Stop failed to cancel startup")
	}
	var calls atomic.Int32
	cancel := handle.cancel
	handle.cancel = func() { calls.Add(1); cancel() }
	handle.serverDone = make(chan struct{})
	close(handle.serverDone)
	manager.finishRun(handle, false) // A late successful acquisition is still cleaned after Stop.
	handle.cleanup(time.Second)
	if calls.Load() != 1 {
		t.Fatalf("cleanup must be exactly once, calls=%d", calls.Load())
	}
	if len(manager.resources) != 0 {
		t.Fatal("timed-out startup leaked registration")
	}
}

func TestManagerConcurrentFinishAndStop(t *testing.T) {
	for i := 0; i < 100; i++ {
		manager := New(t.TempDir(), nil)
		ctx, handle, err := manager.beginRun(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		finished := make(chan struct{})
		go func() { manager.finishRun(handle, false); close(finished) }()
		manager.Stop(time.Second)
		<-finished
		if ctx.Err() == nil {
			t.Fatal("successful handoff remained alive after Stop")
		}
		manager.mu.Lock()
		count := len(manager.resources)
		manager.mu.Unlock()
		if count != 0 {
			t.Fatalf("resources=%d", count)
		}
	}
}
