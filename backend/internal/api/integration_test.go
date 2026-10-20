package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/coder/websocket"
	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
	"websocket_chat_go/backend/internal/svc"
)

func fixture(t *testing.T, start bool) *svc.ServiceContext {
	t.Helper()
	databaseURL, redisAddr := os.Getenv("IM_TEST_DATABASE_URL"), os.Getenv("IM_TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Skip("set IM_TEST_DATABASE_URL and IM_TEST_REDIS_ADDR for integration tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "im_test_" + model.NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	postgres, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	postgres.ConnConfig.RuntimeParams["search_path"] = schema
	settings := config.Config{Postgres: postgres, RedisAddr: redisAddr, Origin: "http://im.test", MigrationsDir: filepath.Join("..", "..", "..", "database", "migrations"), MediaDir: filepath.Join(t.TempDir(), "media"), Heartbeat: 200 * time.Millisecond, HeartbeatTimeout: time.Second}
	services, err := svc.New(ctx, settings)
	if err != nil {
		t.Fatal(err)
	}
	services.Hub = realtime.NewHub(1024)
	workers, cancel := context.WithCancel(ctx)
	t.Cleanup(func() { cancel(); services.Close() })
	if start {
		services.Start(workers)
	}
	return services
}

func account(t *testing.T, services *svc.ServiceContext, name string) (model.User, string) {
	t.Helper()
	user, token, err := services.Auth.Register(t.Context(), logic.Credentials{Email: name + "@example.test", Name: name, Password: "secure-password"})
	if err != nil {
		t.Fatal(err)
	}
	return user, token
}

func status(t *testing.T, err error, expected int) {
	t.Helper()
	var failure *model.Error
	if !errors.As(err, &failure) || failure.Status != expected {
		t.Fatalf("wanted status %d, got %v", expected, err)
	}
}

type wireEvent struct {
	Event     string          `json:"event"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Error     *model.Error    `json:"error"`
}

type peer struct {
	connection *websocket.Conn
	events     chan wireEvent
	done       chan struct{}
}

func dial(t *testing.T, server *httptest.Server, token, device string) *peer {
	t.Helper()
	headers := http.Header{"Origin": {"http://im.test"}, "Cookie": {"session=" + token}}
	connection, _, err := websocket.Dial(t.Context(), "ws"+strings.TrimPrefix(server.URL, "http")+"/ws?device_id="+device, &websocket.DialOptions{HTTPHeader: headers})
	if err != nil {
		t.Fatal(err)
	}
	client := &peer{connection: connection, events: make(chan wireEvent, 1024), done: make(chan struct{})}
	ctx, cancel := context.WithCancel(t.Context())
	t.Cleanup(func() { cancel(); _ = connection.CloseNow(); <-client.done })
	go func() {
		defer close(client.done)
		for {
			_, payload, err := connection.Read(ctx)
			if err != nil {
				return
			}
			var event wireEvent
			if json.Unmarshal(payload, &event) != nil {
				return
			}
			select {
			case client.events <- event:
			case <-ctx.Done():
				return
			}
		}
	}()
	await(t, client, "connection.ready", "")
	return client
}

func await(t *testing.T, client *peer, event, requestID string) wireEvent {
	t.Helper()
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		select {
		case incoming := <-client.events:
			if incoming.Event == event && (requestID == "" || incoming.RequestID == requestID) {
				return incoming
			}
		case <-client.done:
			t.Fatal("connection closed before " + event)
		case <-timer.C:
			t.Fatal("timeout waiting for " + event)
		}
	}
}

func commandTo(t *testing.T, client *peer, event, requestID string, data any) {
	t.Helper()
	payload, err := json.Marshal(model.Event{Event: event, RequestID: requestID, Data: data})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	if err = client.connection.Write(ctx, websocket.MessageText, payload); err != nil {
		t.Fatal(err)
	}
}

func httpCall(t *testing.T, server *httptest.Server, token, method, path string, body any, expected int) json.RawMessage {
	t.Helper()
	payload, _ := json.Marshal(body)
	request, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(payload))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected {
		t.Fatalf("%s: wanted %d got %d: %s", path, expected, response.StatusCode, content)
	}
	var envelope struct {
		Data json.RawMessage `json:"data"`
	}
	if json.Unmarshal(content, &envelope) != nil {
		t.Fatal("invalid response")
	}
	return envelope.Data
}

func noMessages(t *testing.T, client *peer) {
	t.Helper()
	timer := time.NewTimer(300 * time.Millisecond)
	defer timer.Stop()
	for {
		select {
		case event := <-client.events:
			if event.Event == "message.created" {
				t.Fatal("unauthorized fanout")
			}
		case <-timer.C:
			return
		}
	}
}
