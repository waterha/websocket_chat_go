package local

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"
	"websocket_chat_go/desktop/internal/client"
)

// Opt in after prepare-desktop-runtimes.ps1. This uses only the bundled
// binaries with a fresh private cluster and unrelated, free loopback ports.
func TestStandaloneBundledRuntimeFirstLaunchAndRestart(t *testing.T) {
	if os.Getenv("IM_TEST_BUNDLED_RUNTIME") != "1" {
		t.Skip("set IM_TEST_BUNDLED_RUNTIME=1 after preparing release runtimes")
	}
	port := func() int {
		listener, err := net.Listen("tcp", "127.0.0.1:0")
		if err != nil {
			t.Fatal(err)
		}
		value := listener.Addr().(*net.TCPAddr).Port
		listener.Close()
		return value
	}
	pgPort, redisPort, httpPort := port(), port(), port()
	t.Setenv("IM_POSTGRES_BIN", "")
	t.Setenv("IM_REDIS_DIR", "")
	t.Setenv("IM_DESKTOP_PGPORT", strconv.Itoa(pgPort))
	t.Setenv("IM_DESKTOP_REDIS_PORT", strconv.Itoa(redisPort))
	root := filepath.Join(t.TempDir(), "独立桌面 数据 & 测试")
	if err := os.MkdirAll(root, 0755); err != nil {
		t.Fatal(err)
	}
	config := client.Config{ServerURL: fmt.Sprintf("http://127.0.0.1:%d", httpPort), Origin: "http://127.0.0.1:4173"}
	account := map[string]string{"name": "Standalone Test", "email": "standalone@example.test", "password": "standalone-test-password-984"}
	var userID string
	for attempt := 0; attempt < 2; attempt++ {
		manager := New(root, os.DirFS("../../../database/migrations"))
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		started := time.Now()
		err := manager.Run(ctx, config, func(message string) {
			t.Logf("run %d [%s] %s", attempt, time.Since(started).Round(time.Millisecond), message)
		})
		if err != nil {
			for _, name := range []string{"initdb.log", "pg_ctl.log", "postgres.log"} {
				if data, readErr := os.ReadFile(filepath.Join(root, "logs", name)); readErr == nil {
					t.Logf("%s: %s", name, data)
				}
			}
			manager.Stop(15 * time.Second)
			cancel()
			t.Fatal(err)
		}
		func() {
			defer manager.Stop(15 * time.Second)
			defer cancel()
			jar, _ := cookiejar.New(nil)
			httpClient := &http.Client{Jar: jar, Timeout: 15 * time.Second}
			call := func(method, path string, body any) map[string]any {
				var payload []byte
				if body != nil {
					payload, _ = json.Marshal(body)
				}
				request, err := http.NewRequestWithContext(ctx, method, config.ServerURL+"/api/v1"+path, bytes.NewReader(payload))
				if err != nil {
					t.Fatal(err)
				}
				request.Header.Set("Origin", config.Origin)
				request.Header.Set("Content-Type", "application/json")
				response, err := httpClient.Do(request)
				if err != nil {
					t.Fatal(err)
				}
				defer response.Body.Close()
				content, _ := io.ReadAll(io.LimitReader(response.Body, 1<<20))
				if response.StatusCode < 200 || response.StatusCode >= 300 {
					t.Fatalf("%s %s HTTP%d %s", method, path, response.StatusCode, content)
				}
				var envelope map[string]any
				if err := json.Unmarshal(content, &envelope); err != nil {
					t.Fatal(err)
				}
				return envelope
			}
			if attempt == 0 {
				call(http.MethodPost, "/auth/register", account)
			}
			login := call(http.MethodPost, "/auth/login", account)
			data, ok := login["data"].(map[string]any)
			if !ok {
				t.Fatalf("invalid login %v", login)
			}
			if attempt == 0 {
				userID, _ = data["id"].(string)
			} else if data["id"] != userID {
				t.Fatal("persisted user changed after restart")
			}
			if userID == "" {
				t.Fatal("login did not return user")
			}
			call(http.MethodGet, "/me", nil)
			call(http.MethodGet, "/conversations", nil)
			t.Logf("run %d registration/login/session/conversation API verified", attempt)
		}()
		for _, value := range []int{pgPort, redisPort, httpPort} {
			if tcpOpen(value) {
				t.Fatalf("owned service port %d remains after shutdown", value)
			}
		}
	}
}
