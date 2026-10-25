package local

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"websocket_chat_go/desktop/internal/client"
)

func TestPlanDecidesLocalManagement(t *testing.T) {
	tests := []struct {
		name, server string
		wantHost     string
		wantPort     int
		wantOK       bool
	}{
		{"default local", "http://127.0.0.1:8080", "127.0.0.1", 8080, true},
		{"localhost", "http://localhost:9000", "localhost", 9000, true},
		{"bare port", "http://127.0.0.1", "127.0.0.1", 80, true},
		{"remote server", "https://chat.example.com", "", 0, false},
		{"local https", "https://127.0.0.1:8080", "", 0, false},
		{"other host", "http://192.168.1.5:8080", "", 0, false},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, ok := Plan(client.Config{ServerURL: test.server, Origin: test.server})
			if ok != test.wantOK {
				t.Fatalf("Plan(%q) ok = %v, want %v", test.server, ok, test.wantOK)
			}
			if ok && (got.Host != test.wantHost || got.Port != test.wantPort) {
				t.Fatalf("Plan(%q) = %+v, want %s:%d", test.server, got, test.wantHost, test.wantPort)
			}
		})
	}
}

func TestHealthyDetectsRunningChatServer(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.URL.Path != "/healthz" {
			writer.WriteHeader(http.StatusNotFound)
			return
		}
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	target, ok := Plan(client.Config{ServerURL: server.URL, Origin: server.URL})
	if !ok {
		t.Fatal("expected the test server address to be manageable")
	}
	if !healthy(context.Background(), target) {
		t.Fatal("expected the mock chat server to be healthy")
	}
	stopped := Target{Host: "127.0.0.1", Port: 1}
	if healthy(context.Background(), stopped) {
		t.Fatal("expected a closed port to be unhealthy")
	}
}

func TestDirectoryContainingSearchesBinAndMatches(t *testing.T) {
	root := t.TempDir()
	nested := filepath.Join(root, "PostgreSQL 17", "bin")
	if err := os.MkdirAll(nested, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(nested, "pg_ctl.exe"), []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	if got := directoryContaining(root, "pg_ctl.exe", []string{"PostgreSQL*", "*pgsql*"}); got != nested {
		t.Fatalf("directoryContaining = %q, want %q", got, nested)
	}
	if got := directoryContaining(root, "redis-server.exe", nil); got != "" {
		t.Fatalf("directoryContaining found %q for an absent executable", got)
	}
	if got := directoryContaining("", "pg_ctl.exe", nil); got != "" {
		t.Fatalf("empty candidate must not match, got %q", got)
	}
}

func TestLoadOrCreatePasswordPersistsOneSecret(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.txt")
	first, err := loadOrCreatePassword(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != 32 {
		t.Fatalf("generated secret %q has length %d", first, len(first))
	}
	second, err := loadOrCreatePassword(path)
	if err != nil {
		t.Fatal(err)
	}
	if first != second {
		t.Fatalf("secret changed across calls: %q vs %q", first, second)
	}
}

func TestExtractMigrationsCopiesEmbeddedSchema(t *testing.T) {
	source := fstest.MapFS{
		"000001_initial.up.sql":   &fstest.MapFile{Data: []byte("CREATE TABLE sample();")},
		"000001_initial.down.sql": &fstest.MapFile{Data: []byte("DROP TABLE sample;")},
	}
	destination := filepath.Join(t.TempDir(), "migrations")
	if err := extractMigrations(source, destination); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(destination, "000001_initial.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != "CREATE TABLE sample();" {
		t.Fatalf("copied content = %q", data)
	}
	if err := extractMigrations(fstest.MapFS{}, filepath.Join(t.TempDir(), "empty")); err == nil {
		t.Fatal("expected an error when no migrations are embedded")
	}
	if err := extractMigrations(nil, filepath.Join(t.TempDir(), "none")); err == nil {
		t.Fatal("expected an error for a missing migration source")
	}
}

func TestParsePortValidatesRange(t *testing.T) {
	if port, err := parsePort("5434"); err != nil || port != 5434 {
		t.Fatalf("parsePort(5434) = %d, %v", port, err)
	}
	if _, err := parsePort("0"); err == nil {
		t.Fatal("expected port 0 to be rejected")
	}
	if _, err := parsePort("70000"); err == nil {
		t.Fatal("expected port 70000 to be rejected")
	}
	if _, err := parsePort(""); err == nil {
		t.Fatal("expected an empty port to be rejected")
	}
}

func TestRedisPingUnderstandsRESP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	go func() {
		for {
			connection, err := listener.Accept()
			if err != nil {
				return
			}
			go func(connection net.Conn) {
				buffer := make([]byte, 64)
				read, _ := connection.Read(buffer)
				if string(buffer[:read]) == "*1\r\n$4\r\nPING\r\n" {
					connection.Write([]byte("+PONG\r\n"))
				} else {
					connection.Write([]byte("-ERR unknown\r\n"))
				}
				connection.Close()
			}(connection)
		}
	}()
	address := listener.Addr().String()
	if !redisPing(address, time.Second) {
		t.Fatal("expected +PONG to be recognised")
	}
	if redisPing("127.0.0.1:1", 300*time.Millisecond) {
		t.Fatal("expected a closed port to fail the ping")
	}
}

func TestManagerRunSkipsWhenServerAlreadyHealthy(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		writer.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	manager := New(t.TempDir(), fstest.MapFS{})
	var messages []string
	if err := manager.Run(context.Background(), client.Config{ServerURL: server.URL, Origin: server.URL}, func(message string) {
		messages = append(messages, message)
	}); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0], "已在运行") {
		t.Fatalf("progress messages = %v, want a single reuse note", messages)
	}
	// Nothing was started, so stopping must be a no-op that does not hang.
	done := make(chan struct{})
	go func() { manager.Stop(time.Second); close(done) }()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Stop blocked although nothing was started")
	}
}

func TestManagerRunRejectsRemoteServers(t *testing.T) {
	manager := New(t.TempDir(), fstest.MapFS{})
	err := manager.Run(context.Background(), client.Config{ServerURL: "https://chat.example.com", Origin: "https://chat.example.com"}, nil)
	if err == nil {
		t.Fatal("expected remote servers to be rejected")
	}
	var resources []*runHandle
	manager.mu.Lock()
	resources = manager.resources
	manager.mu.Unlock()
	if len(resources) != 0 {
		t.Fatalf("a rejected plan registered %d resources", len(resources))
	}
}

func TestStopShutsDownStartedPostgresAndRedis(t *testing.T) {
	pg := &postgresService{started: true}
	redis := &redisService{started: true}
	handle := &runHandle{pg: pg, redis: redis, done: make(chan error, 1)}
	manager := New(t.TempDir(), fstest.MapFS{})
	manager.resources = []*runHandle{handle}
	manager.Stop(time.Second)
	if pg.started {
		t.Fatal("postgres was not marked stopped")
	}
	if redis.started {
		t.Fatal("redis was not marked stopped")
	}
}

func TestTrimOutputStripsPsqlNewline(t *testing.T) {
	if got := trimOutput([]byte("1\n")); got != "1" {
		t.Fatalf("trimOutput = %q, want %q", got, "1")
	}
	if got := trimOutput([]byte("  hello \r\n")); got != "hello" {
		t.Fatalf("trimOutput = %q, want %q", got, "hello")
	}
	long := strings.Repeat("x", 3000)
	if got := trimOutput([]byte(long)); len(got) != 2000 {
		t.Fatalf("trimOutput kept %d chars, want 2000", len(got))
	}
}
