package local

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
)

func runtimeFixture(t *testing.T) (fstest.MapFS, runtimeSpec) {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("bin/tool.exe")
	if err != nil {
		t.Fatal(err)
	}
	entry.Write([]byte("test-runtime"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(buffer.Bytes())
	spec := runtimeSpec{Version: "16.15-test", Archive: "postgres.zip", SHA256: hex.EncodeToString(sum[:]), Required: []string{"bin/tool.exe"}}
	manifest, _ := json.Marshal(map[string]runtimeSpec{"postgres": spec})
	return fstest.MapFS{"runtimes/manifest.json": &fstest.MapFile{Data: manifest}, "runtimes/postgres.zip": &fstest.MapFile{Data: buffer.Bytes()}}, spec
}

func TestEmbeddedRuntimeInstallReuseRepairAndPreserveUserData(t *testing.T) {
	source, spec := runtimeFixture(t)
	root := t.TempDir()
	user := filepath.Join(root, "pgdata")
	os.MkdirAll(user, 0755)
	os.WriteFile(filepath.Join(user, "keep"), []byte("private-data"), 0600)
	progress := 0
	installed, err := installRuntime(context.Background(), source, root, "postgres", func(string) { progress++ })
	if err != nil {
		t.Fatal(err)
	}
	if !runtimeReady(installed, spec) || progress != 1 {
		t.Fatalf("runtime not ready/progress=%d", progress)
	}
	again, err := installRuntime(context.Background(), source, root, "postgres", func(string) { progress++ })
	if err != nil || again != installed || progress != 1 {
		t.Fatalf("cached reuse: %s %v progress=%d", again, err, progress)
	}
	os.Remove(filepath.Join(installed, "bin", "tool.exe"))
	_, err = installRuntime(context.Background(), source, root, "postgres", nil)
	if err != nil || !runtimeReady(installed, spec) {
		t.Fatalf("repair: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(user, "keep"))
	if err != nil || string(data) != "private-data" {
		t.Fatal("user data modified")
	}
}

func TestEmbeddedRuntimeRejectsCorruptionCancellationAndIncompletePayload(t *testing.T) {
	for _, mode := range []string{"corrupt", "cancel", "missing"} {
		t.Run(mode, func(t *testing.T) {
			source, spec := runtimeFixture(t)
			root := t.TempDir()
			ctx := context.Background()
			switch mode {
			case "corrupt":
				source["runtimes/postgres.zip"].Data = []byte("bad bytes")
			case "cancel":
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			case "missing":
				spec.Required = []string{"bin/missing.exe"}
				manifest, _ := json.Marshal(map[string]runtimeSpec{"postgres": spec})
				source["runtimes/manifest.json"].Data = manifest
			}
			if _, err := installRuntime(ctx, source, root, "postgres", nil); err == nil {
				t.Fatal("unsafe/incomplete runtime accepted")
			}
			entries, _ := os.ReadDir(filepath.Join(root, "runtimes"))
			if len(entries) != 0 {
				t.Fatalf("temporary or installed files remain: %v", entries)
			}
		})
	}
}

func TestRuntimeOverridesAreAuthoritativeAndExistingDataVersionIsProtected(t *testing.T) {
	t.Setenv("IM_POSTGRES_BIN", t.TempDir())
	t.Setenv("IM_REDIS_DIR", t.TempDir())
	if _, err := ensurePostgresBin(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("invalid PG override ignored")
	}
	if _, err := ensureRedisBin(context.Background(), t.TempDir(), nil); err == nil {
		t.Fatal("invalid Redis override ignored")
	}
}
