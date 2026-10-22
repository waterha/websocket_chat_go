package client

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/coder/websocket"
)

func TestConfigValidate(t *testing.T) {
	tests := []struct {
		name, server, origin string
		wantErr              bool
		wantOrigin           string
	}{
		{"http server", "http://127.0.0.1:8080", "http://127.0.0.1:4173/", false, "http://127.0.0.1:4173"},
		{"default origin", "https://chat.example.com/", "", false, "https://chat.example.com"},
		{"reject credentials", "https://user:secret@chat.example.com", "", true, ""},
		{"reject path", "https://chat.example.com/private", "", true, ""},
		{"reject unsafe scheme", "file:///tmp/server", "", true, ""},
		{"reject query", "https://chat.example.com/?token=secret", "", true, ""},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := (Config{ServerURL: test.server, Origin: test.origin}).Validate()
			if (err != nil) != test.wantErr {
				t.Fatalf("Validate() error = %v, wantErr %v", err, test.wantErr)
			}
			if err == nil && got.Origin != test.wantOrigin {
				t.Errorf("origin = %q, want %q", got.Origin, test.wantOrigin)
			}
		})
	}
}

func TestConfigSaveAndLoad(t *testing.T) {
	t.Setenv("IM_DESKTOP_SERVER", "")
	t.Setenv("IM_DESKTOP_ORIGIN", "")
	path := t.TempDir() + "/nested/settings.json"
	if err := Save(path, Config{ServerURL: "https://chat.example.com/", Origin: "https://desktop.example.com/"}); err != nil {
		t.Fatal(err)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	want := Config{ServerURL: "https://chat.example.com", Origin: "https://desktop.example.com"}
	if got != want {
		t.Fatalf("Load() = %#v, want %#v", got, want)
	}
	if err := Save(path, Config{ServerURL: "file:///etc/passwd"}); err == nil {
		t.Fatal("Save accepted an unsafe server URL")
	}
	got, err = Load(path)
	if err != nil {
		t.Fatalf("a rejected replacement must retain the saved configuration: %v", err)
	}
	if got != want {
		t.Fatalf("rejected save changed settings: %#v, want %#v", got, want)
	}
}

func TestClientStreamsUploadWithNativeCookiesAndCSRF(t *testing.T) {
	var uploaded atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v1/auth/login" {
			http.SetCookie(w, &http.Cookie{Name: "session", Value: strings.Repeat("a", 64), Path: "/", HttpOnly: true})
			http.SetCookie(w, &http.Cookie{Name: "csrf", Value: "private-token", Path: "/"})
			w.WriteHeader(http.StatusOK)
			return
		}
		if r.URL.Path != "/api/v1/conversations/conversation/media" {
			http.Error(w, "wrong API path", http.StatusNotFound)
			return
		}
		if r.Header.Get("Origin") != "http://localhost:4173" {
			http.Error(w, "wrong origin", http.StatusForbidden)
			return
		}
		if r.Header.Get("X-CSRF-Token") != "private-token" {
			http.Error(w, "wrong CSRF", http.StatusForbidden)
			return
		}
		if r.Header.Get("X-Upload-Size") != "" {
			http.Error(w, "desktop-only header leaked upstream", http.StatusBadRequest)
			return
		}
		data, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		uploaded.Store(int64(len(data)))
		w.WriteHeader(http.StatusCreated)
	}))
	defer server.Close()
	c, err := New(Config{ServerURL: server.URL, Origin: "http://localhost:4173"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	login, err := c.request(t.Context(), http.MethodPost, "/api/v1/auth/login", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	login.Body.Close()
	const size = 3 << 20
	body := bytes.NewReader(bytes.Repeat([]byte("a"), size))
	local := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/api/v1/conversations/conversation/media?file_name=chat.png", body)
	request.ContentLength = size
	request.Header.Set("Content-Type", "image/png")
	c.ServeHTTP(local, request)
	if local.Code != http.StatusCreated {
		t.Fatalf("upload status %d: %s", local.Code, local.Body.String())
	}
	if uploaded.Load() != size {
		t.Fatalf("server received %d bytes, want %d", uploaded.Load(), size)
	}
}

func TestOpenSocketPreservesAuthOriginAndEvents(t *testing.T) {
	const origin = "http://localhost:4173"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Origin") != origin {
			http.Error(w, "wrong origin", http.StatusForbidden)
			return
		}
		conn, err := websocket.Accept(w, r, &websocket.AcceptOptions{OriginPatterns: []string{"localhost:4173"}})
		if err != nil {
			return
		}
		defer conn.CloseNow()
		_ = conn.Write(r.Context(), websocket.MessageText, []byte(`{"event":"connection.ready"}`))
		_, _, _ = conn.Read(r.Context())
	}))
	defer server.Close()
	c, err := New(Config{ServerURL: server.URL, Origin: origin})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	events := make(chan SocketEvent, 3)
	if err := c.OpenSocket("socket-1", "device-1", func(e SocketEvent) { events <- e }); err != nil {
		t.Fatal(err)
	}
	wait := func(want string) {
		t.Helper()
		select {
		case got := <-events:
			if got.Type != want {
				t.Fatalf("event type %q, want %q", got.Type, want)
			}
		case <-time.After(3 * time.Second):
			t.Fatalf("timed out waiting for %q event", want)
		}
	}
	wait("open")
	wait("message")
	if err := c.SendSocket("socket-1", `{"event":"ping"}`); err != nil {
		t.Fatal(err)
	}
	c.CloseSocket("socket-1")
	wait("close")
}

func TestDownloadStreamsAndPreservesExistingFileOnFailure(t *testing.T) {
	const content = "authenticated-download"
	var revoked atomic.Bool
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/v1/media/media-1" {
			http.NotFound(w, r)
			return
		}
		if r.Header.Get("Origin") != "http://localhost:4173" {
			http.Error(w, "wrong origin", http.StatusForbidden)
			return
		}
		if revoked.Load() {
			http.Error(w, "revoked", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, content)
	}))
	defer server.Close()
	c, err := New(Config{ServerURL: server.URL, Origin: "http://localhost:4173"})
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	dir := t.TempDir()
	if err := c.Download(context.Background(), "media-1", dir+"/saved.png"); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(dir + "/saved.png")
	if err != nil || string(data) != content {
		t.Fatalf("download = %q, %v", data, err)
	}
	if err := os.WriteFile(dir+"/saved.png", []byte("keep-existing-file"), 0600); err != nil {
		t.Fatal(err)
	}
	revoked.Store(true)
	if err := c.Download(context.Background(), "media-1", dir+"/saved.png"); err == nil {
		t.Fatal("a revoked attachment was reported as downloaded")
	}
	data, err = os.ReadFile(dir + "/saved.png")
	if err != nil || string(data) != "keep-existing-file" {
		t.Fatalf("failed download modified the existing file: %q, %v", data, err)
	}
}
