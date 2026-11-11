package main

import (
	"context"
	"errors"
	"reflect"
	"sync/atomic"
	"testing"
	"testing/fstest"
	"time"

	"websocket_chat_go/desktop/internal/client"
)

type progressTestManager struct {
	messages []string
	err      error
}

func (m *progressTestManager) Run(_ context.Context, _ client.Config, progress func(string)) error {
	for _, message := range m.messages {
		progress(message)
	}
	return m.err
}
func (m *progressTestManager) Stop(time.Duration) {}

func TestBootstrapEmitsEveryProgressAndTerminalState(t *testing.T) {
	for _, failed := range []bool{false, true} {
		t.Run(map[bool]string{false: "ready", true: "failed"}[failed], func(t *testing.T) {
			manager := &progressTestManager{messages: []string{"解压数据库", "初始化数据库", "准备Redis", "更新数据库结构"}}
			if failed {
				manager.err = errors.New("测试启动失败")
			}
			var events []bootstrapState
			app := &App{manager: manager, bootstrapEvent: func(state bootstrapState) { events = append(events, state) }}
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			app.runBootstrap(ctx, cancel, client.Config{ServerURL: "http://127.0.0.1:8080"})
			if len(events) != len(manager.messages)+1 {
				t.Fatalf("events=%+v", events)
			}
			var messages []string
			for _, event := range events[:len(manager.messages)] {
				if event.Stage != "running" {
					t.Fatalf("progress stage=%s", event.Stage)
				}
				messages = append(messages, event.Message)
			}
			if !reflect.DeepEqual(messages, manager.messages) {
				t.Fatalf("progress=%v", messages)
			}
			want := "ready"
			if failed {
				want = "failed"
			}
			if events[len(events)-1].Stage != want {
				t.Fatalf("last=%+v", events[len(events)-1])
			}
			if failed && app.BootstrapStatus().Message != "测试启动失败" {
				t.Fatal("failure detail was not preserved")
			}
		})
	}
}

func TestBootstrapIgnoresCancelledOrClosedProgress(t *testing.T) {
	events := 0
	app := &App{bootstrapEvent: func(bootstrapState) { events++ }}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	app.publishBootstrap(ctx, bootstrapState{Stage: "running", Message: "过期进度"})
	app.stopped = true
	app.publishBootstrap(context.Background(), bootstrapState{Stage: "ready"})
	if events != 0 || app.BootstrapStatus().Stage != "" {
		t.Fatal("late progress updated a cancelled or closed window")
	}
}

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
