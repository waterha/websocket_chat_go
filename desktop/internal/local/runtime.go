package local

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
)

type runtimeSpec struct {
	Version  string   `json:"version"`
	Archive  string   `json:"archive"`
	SHA256   string   `json:"sha256"`
	Required []string `json:"required"`
}

var runtimeInstallGate = make(chan struct{}, 1)
var runtimeVersionPattern = regexp.MustCompile(`^[0-9][A-Za-z0-9._-]*$`)

func ensurePostgresBin(ctx context.Context, root string, progress func(string)) (string, error) {
	major := ""
	if data, err := os.ReadFile(filepath.Join(root, "pgdata", "PG_VERSION")); err == nil {
		major = strings.TrimSpace(string(data))
	} else if !os.IsNotExist(err) {
		return "", err
	}
	if override := os.Getenv("IM_POSTGRES_BIN"); override != "" {
		bin := directoryContaining(override, exeName("pg_ctl"), nil)
		if bin == "" {
			return "", fmt.Errorf("IM_POSTGRES_BIN 中没有 pg_ctl，请修正该高级设置")
		}
		return compatiblePostgres(ctx, bin, major)
	}
	// Existing clusters of another major must never be silently opened/upgraded.
	// The bundled PG16 minor update can safely reuse our previous PG16 data.
	if major != "" && major != "16" {
		if bin := FindPostgres(); bin != "" {
			return compatiblePostgres(ctx, bin, major)
		}
		return "", fmt.Errorf("已有本机数据使用 PostgreSQL %s；内置运行时为16，未改动原数据，请连接原服务或配置对应版本运行时", major)
	}
	directory, err := installRuntime(ctx, runtimeAssets, root, "postgres", progress)
	if err != nil {
		return "", err
	}
	return compatiblePostgres(ctx, filepath.Join(directory, "bin"), major)
}

func compatiblePostgres(ctx context.Context, bin, major string) (string, error) {
	for _, name := range []string{"pg_ctl", "postgres", "initdb", "psql"} {
		if !fileExists(filepath.Join(bin, exeName(name))) {
			return "", fmt.Errorf("数据库运行时不完整：缺少 %s", name)
		}
	}
	command := exec.CommandContext(ctx, filepath.Join(bin, exeName("postgres")), "--version")
	hideWindow(command)
	output, err := command.Output()
	if err != nil {
		return "", fmt.Errorf("无法启动内置数据库程序：%w", err)
	}
	fields := strings.Fields(string(output))
	if len(fields) == 0 {
		return "", fmt.Errorf("无法识别数据库版本")
	}
	version := strings.Split(fields[len(fields)-1], ".")[0]
	if major != "" && version != major {
		return "", fmt.Errorf("数据库版本 %s 与已有数据版本 %s 不匹配；原数据未改动", version, major)
	}
	return bin, nil
}

func ensureRedisBin(ctx context.Context, root string, progress func(string)) (string, error) {
	if override := os.Getenv("IM_REDIS_DIR"); override != "" {
		if bin := directoryContaining(override, exeName("redis-server"), []string{"*redis*"}); bin != "" {
			return bin, nil
		}
		return "", fmt.Errorf("IM_REDIS_DIR 中没有 redis-server，请修正该高级设置")
	}
	directory, err := installRuntime(ctx, runtimeAssets, root, "redis", progress)
	if err != nil {
		return "", err
	}
	return directory, nil
}

func installRuntime(ctx context.Context, source fs.FS, root, kind string, progress func(string)) (string, error) {
	select {
	case runtimeInstallGate <- struct{}{}:
		defer func() { <-runtimeInstallGate }()
	case <-ctx.Done():
		return "", ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	data, err := fs.ReadFile(source, "runtimes/manifest.json")
	if err != nil {
		return "", fmt.Errorf("此程序没有内置便携运行时，请使用完整构建的 WebsocketChat.exe：%w", err)
	}
	var manifest map[string]runtimeSpec
	if err := json.Unmarshal(data, &manifest); err != nil {
		return "", fmt.Errorf("运行时清单无效：%w", err)
	}
	spec, ok := manifest[kind]
	if !ok || !runtimeVersionPattern.MatchString(spec.Version) || filepath.Base(spec.Archive) != spec.Archive || len(spec.Required) == 0 {
		return "", fmt.Errorf("运行时 %s 清单不完整", kind)
	}
	expected, err := hex.DecodeString(spec.SHA256)
	if err != nil || len(expected) != sha256.Size {
		return "", fmt.Errorf("运行时 %s 校验值无效", kind)
	}
	base := filepath.Join(root, "runtimes")
	if err := os.MkdirAll(base, 0755); err != nil {
		return "", err
	}
	destination := filepath.Join(base, kind+"-"+spec.Version+"-"+spec.SHA256[:12])
	if runtimeReady(destination, spec) {
		return destination, nil
	}
	if progress != nil {
		progress(fmt.Sprintf("首次运行：正在释放内置 %s 运行时，无需联网或手动安装", kind))
	}
	staging, err := os.MkdirTemp(base, ".prepare-"+kind+"-")
	if err != nil {
		return "", err
	}
	defer os.RemoveAll(staging)
	archive, err := os.CreateTemp(base, ".runtime-*.zip")
	if err != nil {
		return "", err
	}
	archiveName := archive.Name()
	defer os.Remove(archiveName)
	input, err := source.Open("runtimes/" + spec.Archive)
	if err != nil {
		archive.Close()
		return "", err
	}
	hash := sha256.New()
	_, err = io.Copy(io.MultiWriter(archive, hash), contextReader{ctx: ctx, source: input})
	input.Close()
	closeErr := archive.Close()
	if err != nil {
		return "", err
	}
	if closeErr != nil {
		return "", closeErr
	}
	if hex.EncodeToString(hash.Sum(nil)) != spec.SHA256 {
		return "", fmt.Errorf("内置 %s 运行时校验失败，未执行或安装", kind)
	}
	if err := unpackZipContext(ctx, archiveName, staging); err != nil {
		return "", err
	}
	for _, path := range spec.Required {
		if !fs.ValidPath(path) || !fileExists(filepath.Join(staging, filepath.FromSlash(path))) {
			return "", fmt.Errorf("内置 %s 运行时不完整：%s", kind, path)
		}
	}
	if err := ctx.Err(); err != nil {
		return "", err
	}
	if err := os.WriteFile(filepath.Join(staging, ".runtime-ready"), []byte(spec.SHA256), 0644); err != nil {
		return "", err
	}
	// Only replace our exact version/hash-specific generated runtime directory,
	// never pgdata, user settings, external installations, or other versions.
	if err := os.RemoveAll(destination); err != nil {
		return "", err
	}
	if err := os.Rename(staging, destination); err != nil {
		return "", err
	}
	return destination, nil
}

func runtimeReady(directory string, spec runtimeSpec) bool {
	data, err := os.ReadFile(filepath.Join(directory, ".runtime-ready"))
	if err != nil || string(data) != spec.SHA256 {
		return false
	}
	for _, path := range spec.Required {
		if !fs.ValidPath(path) || !fileExists(filepath.Join(directory, filepath.FromSlash(path))) {
			return false
		}
	}
	return true
}
