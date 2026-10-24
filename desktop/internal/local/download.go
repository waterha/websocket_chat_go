package local

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// redisDownloadURL matches the portable build used by scripts/dev-env.ps1.
const redisDownloadURL = "https://github.com/redis-windows/redis-windows/releases/download/8.10.2/Redis-8.10.2-Windows-x64-cygwin.zip"

// downloadRedis fetches the portable Redis archive into root and unpacks it
// under root/redis, mirroring the first-run experience of the dev launcher.
func downloadRedis(ctx context.Context, root string) error {
	destination := filepath.Join(root, "redis")
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	archive := filepath.Join(root, "redis.zip")
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, redisDownloadURL, nil)
	if err != nil {
		return err
	}
	progressClient := &http.Client{Timeout: 10 * time.Minute}
	response, err := progressClient.Do(request)
	if err != nil {
		return fmt.Errorf("下载便携版 Redis 失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载便携版 Redis 失败：HTTP %d", response.StatusCode)
	}
	file, err := os.Create(archive)
	if err != nil {
		return err
	}
	if _, err = io.Copy(file, response.Body); err != nil {
		file.Close()
		return fmt.Errorf("下载便携版 Redis 失败：%w", err)
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = unpackZip(archive, destination); err != nil {
		return err
	}
	return os.Remove(archive)
}

func unpackZip(archive, destination string) error {
	reader, err := zip.OpenReader(archive)
	if err != nil {
		return fmt.Errorf("解压便携版 Redis 失败：%w", err)
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if err := unpackEntry(entry, destination); err != nil {
			return err
		}
	}
	return nil
}

func unpackEntry(entry *zip.File, destination string) error {
	name := filepath.Clean(entry.Name)
	if strings.HasPrefix(name, "..") || filepath.IsAbs(name) {
		return fmt.Errorf("压缩包包含不安全的路径 %q", entry.Name)
	}
	target := filepath.Join(destination, name)
	if entry.FileInfo().IsDir() {
		return os.MkdirAll(target, 0755)
	}
	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return err
	}
	source, err := entry.Open()
	if err != nil {
		return err
	}
	defer source.Close()
	targetFile, err := os.OpenFile(target, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, entry.Mode())
	if err != nil {
		return err
	}
	if _, err = io.Copy(targetFile, source); err != nil {
		targetFile.Close()
		return err
	}
	return targetFile.Close()
}
