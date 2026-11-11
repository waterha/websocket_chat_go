package local

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type runtimeZipEntry struct {
	name, body string
	mode       os.FileMode
}

func runtimeZip(t *testing.T, entries ...runtimeZipEntry) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	for _, entry := range entries {
		header := &zip.FileHeader{Name: entry.name, Method: zip.Deflate}
		if entry.mode != 0 {
			header.SetMode(entry.mode)
		}
		target, err := writer.CreateHeader(header)
		if err != nil {
			t.Fatal(err)
		}
		if _, err = io.WriteString(target, entry.body); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
func TestUnpackRuntimeRejectsUnsafePaths(t *testing.T) {
	for _, name := range []string{"../escape", "bin/../../escape", "bin/../escape", "..\\escape", "bin\\..\\escape", "/absolute", "C:/absolute", "C:relative", "\\\\server\\share\\file", "\\rooted", "bin/redis.exe:payload", "bin/file:stream:$DATA", "bin/NUL.txt", "lib/COM1", "share/end. ", "bin/./file", "bin//file", "bin/CONOUT$"} {
		t.Run(name, func(t *testing.T) {
			destination := filepath.Join(t.TempDir(), "runtime")
			data := runtimeZip(t, runtimeZipEntry{name: name, body: "attack"})
			if err := unpackZipReaderContext(context.Background(), bytes.NewReader(data), int64(len(data)), destination); err == nil {
				t.Fatalf("accepted unsafe path %q", name)
			}
			if _, err := os.Stat(destination); !os.IsNotExist(err) {
				t.Fatalf("unsafe headers created destination: %v", err)
			}
		})
	}
}
func TestUnpackRuntimeNormalDirectoriesAndFileHelper(t *testing.T) {
	data := runtimeZip(t, runtimeZipEntry{name: "runtimebin/", mode: os.ModeDir | 0755}, runtimeZipEntry{name: "runtimebin/postgres.exe", body: "binary"}, runtimeZipEntry{name: "lib\\postgresql\\module.dll", body: "lib"}, runtimeZipEntry{name: "share/timezonesets/Default", body: "zone"})
	root := t.TempDir()
	archive := filepath.Join(root, "runtime.zip")
	if err := os.WriteFile(archive, data, 0600); err != nil {
		t.Fatal(err)
	}
	destination := filepath.Join(root, "installed")
	if err := unpackZip(archive, destination); err != nil {
		t.Fatal(err)
	}
	for _, file := range []string{"runtimebin/postgres.exe", "lib/postgresql/module.dll", "share/timezonesets/Default"} {
		if _, err := os.Stat(filepath.Join(destination, file)); err != nil {
			t.Fatal(err)
		}
	}
}
func TestUnpackRuntimeRejectsSymlinkEntries(t *testing.T) {
	data := runtimeZip(t, runtimeZipEntry{name: "bin/link", body: "../../outside", mode: os.ModeSymlink | 0777})
	if err := unpackZipReaderContext(context.Background(), bytes.NewReader(data), int64(len(data)), filepath.Join(t.TempDir(), "runtime")); err == nil {
		t.Fatal("accepted zip symlink")
	}
}
func TestUnpackRuntimeRejectsSymlinkParents(t *testing.T) {
	root := t.TempDir()
	outside := t.TempDir()
	destination := filepath.Join(root, "runtime")
	if err := os.Mkdir(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(destination, "bin")); err != nil {
		t.Skipf("OS disallows creating symlinks: %v", err)
	}
	data := runtimeZip(t, runtimeZipEntry{name: "bin/escape", body: "attack"})
	if err := unpackZipReaderContext(context.Background(), bytes.NewReader(data), int64(len(data)), destination); err == nil {
		t.Fatal("accepted symlink parent")
	}
	if _, err := os.Stat(filepath.Join(outside, "escape")); !os.IsNotExist(err) {
		t.Fatal("wrote outside destination")
	}
	data = runtimeZip(t, runtimeZipEntry{name: "safe", body: "attack"})
	if err := unpackZipReaderContext(context.Background(), bytes.NewReader(data), int64(len(data)), filepath.Join(destination, "bin", "child")); err == nil {
		t.Fatal("accepted symlink destination ancestor")
	}
}
func TestUnpackRuntimeRejectsDuplicateAndOversizedHeaders(t *testing.T) {
	data := runtimeZip(t, runtimeZipEntry{name: "bin/Tool.exe", body: "one"}, runtimeZipEntry{name: "bin/tool.exe", body: "two"})
	destination := filepath.Join(t.TempDir(), "runtime")
	if err := unpackZipReaderContext(context.Background(), bytes.NewReader(data), int64(len(data)), destination); err == nil {
		t.Fatal("accepted case alias")
	}
	if err := unpackZipReaderContext(context.Background(), bytes.NewReader(nil), maxRuntimeArchiveBytes+1, destination); err == nil {
		t.Fatal("accepted oversized archive")
	}
	entry := zip.FileHeader{Name: "lib/huge", Method: zip.Store, UncompressedSize64: maxRuntimeFileBytes + 1, CompressedSize64: maxRuntimeFileBytes + 1}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	if _, err := writer.CreateRaw(&entry); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := unpackZipReaderContext(context.Background(), bytes.NewReader(buffer.Bytes()), int64(buffer.Len()), destination); err == nil {
		t.Fatal("accepted oversized header")
	}
}

type cancelZipReader struct {
	*bytes.Reader
	cancel context.CancelFunc
	reads  int
}

func (r *cancelZipReader) ReadAt(p []byte, off int64) (int, error) {
	r.reads++
	if r.reads == 3 {
		r.cancel()
	}
	return r.Reader.ReadAt(p, off)
}
func TestUnpackRuntimeCancellation(t *testing.T) {
	data := runtimeZip(t, runtimeZipEntry{name: "bin/server.exe", body: strings.Repeat("runtime", 10000)})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	destination := filepath.Join(t.TempDir(), "runtime")
	if err := unpackZipReaderContext(ctx, bytes.NewReader(data), int64(len(data)), destination); !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-canceled: %v", err)
	}
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	reader := &cancelZipReader{Reader: bytes.NewReader(data), cancel: cancel}
	if err := unpackZipReaderContext(ctx, reader, int64(len(data)), destination); !errors.Is(err, context.Canceled) {
		t.Fatalf("mid-read cancel: %v", err)
	}
}

type runtimeRoundTrip func(*http.Request) (*http.Response, error)

func (f runtimeRoundTrip) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func TestDownloadRedisAtomicInstallAndCleanup(t *testing.T) {
	previous := http.DefaultTransport
	defer func() { http.DefaultTransport = previous }()
	root := t.TempDir()
	destination := filepath.Join(root, "redis")
	if err := os.Mkdir(destination, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "old.exe"), []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	data := runtimeZip(t, runtimeZipEntry{name: "redis-server.exe", body: "new"})
	http.DefaultTransport = runtimeRoundTrip(func(*http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data)), ContentLength: int64(len(data))}, nil
	})
	if err := downloadRedis(context.Background(), root); err != nil {
		t.Fatal(err)
	}
	if body, err := os.ReadFile(filepath.Join(destination, "redis-server.exe")); err != nil || string(body) != "new" {
		t.Fatalf("new installation missing: %s %v", body, err)
	}
	assertNoRuntimeTemps(t, root)
	data = runtimeZip(t, runtimeZipEntry{name: "../escape", body: "bad"})
	if err := downloadRedis(context.Background(), root); err == nil {
		t.Fatal("unsafe download succeeded")
	}
	if _, err := os.Stat(filepath.Join(destination, "redis-server.exe")); err != nil {
		t.Fatalf("failed download damaged existing install: %v", err)
	}
	assertNoRuntimeTemps(t, root)
	ctx, cancel := context.WithCancel(context.Background())
	http.DefaultTransport = runtimeRoundTrip(func(*http.Request) (*http.Response, error) {
		cancel()
		return &http.Response{StatusCode: 200, Body: io.NopCloser(bytes.NewReader(data))}, nil
	})
	if err := downloadRedis(ctx, root); !errors.Is(err, context.Canceled) {
		t.Fatalf("download cancellation: %v", err)
	}
	assertNoRuntimeTemps(t, root)
}
func assertNoRuntimeTemps(t *testing.T, root string) {
	t.Helper()
	entries, err := os.ReadDir(root)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if strings.HasPrefix(entry.Name(), ".redis-") {
			t.Errorf("temporary download/install remains: %s", entry.Name())
		}
	}
}
