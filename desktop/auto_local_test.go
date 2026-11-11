package main

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"websocket_chat_go/desktop/internal/client"
)

func TestFirstLaunchAvoidsForeignLocalPortWithoutOverwritingExplicitSettings(t *testing.T) {
	t.Setenv("IM_DESKTOP_SERVER", "")
	foreign := httptest.NewServer(http.NotFoundHandler())
	defer foreign.Close()
	config := client.Config{ServerURL: foreign.URL, Origin: "http://127.0.0.1:4173"}
	path := filepath.Join(t.TempDir(), "settings.json")
	next, err := automaticLocalConfig(path, config)
	if err != nil {
		t.Fatal(err)
	}
	if next.ServerURL == config.ServerURL || next.Origin != config.Origin {
		t.Fatalf("fallback not selected: %+v", next)
	}
	saved, err := client.Load(path)
	if err != nil || saved != next {
		t.Fatalf("automatic address not persisted: %+v %v", saved, err)
	}
	// A configured address is deliberate and must not be changed by probing.
	explicit := filepath.Join(t.TempDir(), "settings.json")
	os.WriteFile(explicit, []byte(`{}`), 0600)
	actual, err := automaticLocalConfig(explicit, config)
	if err != nil || actual != config {
		t.Fatalf("explicit setting changed: %+v %v", actual, err)
	}
	t.Setenv("IM_DESKTOP_SERVER", foreign.URL)
	actual, err = automaticLocalConfig(filepath.Join(t.TempDir(), "settings.json"), config)
	if err != nil || actual != config {
		t.Fatal("environment override changed")
	}
}

func TestFirstLaunchReusesReadyLocalServiceAndLeavesRemoteServerAlone(t *testing.T) {
	t.Setenv("IM_DESKTOP_SERVER", "")
	ready := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }))
	defer ready.Close()
	for _, address := range []string{ready.URL, "https://team.example.test"} {
		config := client.Config{ServerURL: address, Origin: "http://127.0.0.1:4173"}
		path := filepath.Join(t.TempDir(), "settings.json")
		actual, err := automaticLocalConfig(path, config)
		if err != nil || actual != config {
			t.Fatalf("valid target changed: %+v %v", actual, err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Fatal("unnecessary setting file written")
		}
	}
}
