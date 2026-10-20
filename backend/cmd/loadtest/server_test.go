package main

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// bootServer starts the isolated test server and fails the test if it does not
// become ready. Returning the fixture lets a caller talk to it.
func bootServer(t *testing.T, members int) (fixture, func()) {
	t.Helper()
	if os.Getenv("IM_TEST_DATABASE_URL") == "" || os.Getenv("IM_TEST_REDIS_ADDR") == "" {
		t.Skip("set IM_TEST_DATABASE_URL and IM_TEST_REDIS_ADDR")
	}
	directory := t.TempDir()
	fixturePath := filepath.Join(directory, "credentials.json")
	settings := options{
		mode:       "server",
		address:    "127.0.0.1:18099",
		fixture:    fixturePath,
		members:    members,
		// The test runs in cmd/loadtest, so the migrations directory is three
		// levels up from the working directory.
		migrations: filepath.Join("..", "..", "..", "database", "migrations"),
	}
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, settings) }()
	// Wait for the fixture file, which is written only after the schema is
	// migrated, the accounts are seeded and the listener is bound.
	deadline := time.Now().Add(60 * time.Second)
	for !fileExists(fixturePath) {
		if time.Now().After(deadline) {
			cancel()
			t.Fatal("the test server never became ready")
		}
		select {
		case err := <-done:
			t.Fatalf("the test server exited early: %v", err)
		default:
		}
		time.Sleep(150 * time.Millisecond)
	}
	content, err := os.ReadFile(fixturePath)
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	var data fixture
	if err = json.Unmarshal(content, &data); err != nil {
		cancel()
		t.Fatal(err)
	}
	cleanup := func() {
		// Ask the server to stop, then cancel so the goroutine always ends.
		request, err := http.NewRequestWithContext(context.Background(), "POST", data.URL+"/_loadtest/stop", nil)
		if err == nil {
			request.Header.Set("X-Loadtest-Token", data.AdminToken)
			if response, requestErr := http.DefaultClient.Do(request); requestErr == nil {
				response.Body.Close()
			}
		}
		cancel()
		select {
		case <-done:
		case <-time.After(30 * time.Second):
			t.Error("the test server did not shut down")
		}
	}
	return data, cleanup
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir()
}

// TestServeStartsWithSeededSessionAndCSRF guards the exact regressions that once
// made this tool unable to start at all: a missing media directory in the
// service config, and seeded sessions without a CSRF token.
func TestServeStartsWithSeededSessionAndCSRF(t *testing.T) {
	data, cleanup := bootServer(t, 3)
	defer cleanup()
	if data.Group == "" || len(data.Accounts) != 3 {
		t.Fatalf("fixture is incomplete: %+v", data)
	}
	for _, account := range data.Accounts {
		if account.UserID == "" || len(account.Token) != 64 || account.CSRFToken == "" {
			t.Fatalf("seeded account lacks a usable session: %+v", account)
		}
	}
}

// A state-changing request without the token must be refused, and the same
// request with it must succeed, proving the seeded token is the accepted one.
func TestServeAcceptsCSRFTokenOnWrites(t *testing.T) {
	data, cleanup := bootServer(t, 3)
	defer cleanup()
	account := data.Accounts[1]

	request, err := http.NewRequestWithContext(t.Context(), "POST", data.URL+"/api/v1/conversations/"+data.Group+"/received", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+account.Token)
	request.Header.Set("Origin", data.Origin)
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if response.StatusCode != 403 {
		t.Fatalf("a write without a CSRF token must be 403, got %d", response.StatusCode)
	}

	body := `{"device_id":"smoke","seq":0}`
	request, err = http.NewRequestWithContext(t.Context(), "POST", data.URL+"/api/v1/conversations/"+data.Group+"/received", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+account.Token)
	request.Header.Set("Origin", data.Origin)
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("X-CSRF-Token", account.CSRFToken)
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != 200 {
		t.Fatalf("a write with the seeded CSRF token must succeed, got %d", response.StatusCode)
	}
}
