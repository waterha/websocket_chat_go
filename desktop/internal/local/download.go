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

const (
	maxRuntimeArchiveBytes  int64  = 1 << 30
	maxRuntimeFileBytes     uint64 = 1 << 30
	maxRuntimeExpandedBytes uint64 = 4 << 30
	maxRuntimeEntries              = 100000
)

// Download and extract privately. Only a fully validated installation is renamed
// into place; cancellation and errors never expose a partial new runtime.
func downloadRedis(ctx context.Context, root string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := safeDirectory(root); err != nil {
		return err
	}
	file, err := os.CreateTemp(root, ".redis-download-*.zip")
	if err != nil {
		return err
	}
	archive := file.Name()
	defer os.Remove(archive)
	defer file.Close()
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, redisDownloadURL, nil)
	if err != nil {
		return err
	}
	client := &http.Client{Timeout: 10 * time.Minute}
	response, err := client.Do(request)
	if err != nil {
		return fmt.Errorf("下载便携版 Redis 失败：%w", err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("下载便携版 Redis 失败：HTTP %d", response.StatusCode)
	}
	if response.ContentLength > maxRuntimeArchiveBytes {
		return fmt.Errorf("便携运行时压缩包过大")
	}
	copied, err := io.Copy(file, io.LimitReader(contextReader{ctx, response.Body}, maxRuntimeArchiveBytes+1))
	if err != nil {
		return fmt.Errorf("下载便携版 Redis 失败：%w", err)
	}
	if copied > maxRuntimeArchiveBytes {
		return fmt.Errorf("便携运行时压缩包过大")
	}
	if err = file.Close(); err != nil {
		return err
	}
	staging, err := os.MkdirTemp(root, ".redis-install-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(staging) // staging is the unique directory created above.
	if err = unpackZipContext(ctx, archive, staging); err != nil {
		return err
	}
	if err = ctx.Err(); err != nil {
		return err
	}
	destination := filepath.Join(root, "redis")
	backup := staging + "-previous"
	replaced := false
	if info, e := os.Lstat(destination); e == nil {
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return fmt.Errorf("Redis 安装路径不是安全目录")
		}
		if err = os.Rename(destination, backup); err != nil {
			return err
		}
		replaced = true
	} else if !os.IsNotExist(e) {
		return e
	}
	if err = os.Rename(staging, destination); err != nil {
		if replaced {
			if rollback := os.Rename(backup, destination); rollback != nil {
				return fmt.Errorf("安装失败：%v；恢复旧目录失败（保留于 %s）：%w", err, backup, rollback)
			}
		}
		return err
	}
	if replaced {
		return os.RemoveAll(backup)
	} // backup only contains the prior installation we renamed.
	return nil
}

func unpackZip(archive, destination string) error {
	return unpackZipContext(context.Background(), archive, destination)
}

func unpackZipContext(ctx context.Context, archive, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.Open(archive)
	if err != nil {
		return err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return err
	}
	return unpackZipReaderContext(ctx, file, info.Size(), destination)
}

// unpackZipReaderContext accepts embedded ZIP files without a disk copy. The
// caller owns source. A canceled/error extraction may leave files in destination;
// install into a private staging directory and rename only after success.
func unpackZipReaderContext(ctx context.Context, source io.ReaderAt, size int64, destination string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if size < 0 || size > maxRuntimeArchiveBytes {
		return fmt.Errorf("便携运行时压缩包大小超限")
	}
	reader, err := zip.NewReader(contextReaderAt{ctx, source}, size)
	if err != nil {
		return fmt.Errorf("解压便携运行时失败：%w", err)
	}
	if len(reader.File) > maxRuntimeEntries {
		return fmt.Errorf("压缩包文件数超限")
	}
	var expanded uint64
	names := make(map[string]bool, len(reader.File))
	// Validate every header before creating files, including platform-specific
	// Windows aliases even when these tests run on Linux/macOS.
	for _, entry := range reader.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		name, e := runtimeEntryName(entry)
		if e != nil {
			return e
		}
		key := strings.ToLower(name)
		if names[key] {
			return fmt.Errorf("压缩包路径重复 %q", entry.Name)
		}
		names[key] = true
		if entry.UncompressedSize64 > maxRuntimeFileBytes || entry.UncompressedSize64 > maxRuntimeExpandedBytes-expanded {
			return fmt.Errorf("压缩包展开大小超限")
		}
		expanded += entry.UncompressedSize64
	}
	if err = safeDirectory(destination); err != nil {
		return err
	}
	root, err := os.OpenRoot(destination)
	if err != nil {
		return err
	}
	defer root.Close()
	for _, entry := range reader.File {
		if err = ctx.Err(); err != nil {
			return err
		}
		name, _ := runtimeEntryName(entry)
		target := filepath.Join(destination, filepath.FromSlash(name))
		if entry.FileInfo().IsDir() {
			if err = safeDirectory(target); err != nil {
				return err
			}
			continue
		}
		if err = safeDirectory(filepath.Dir(target)); err != nil {
			return err
		}
		if _, e := os.Lstat(target); e == nil {
			return fmt.Errorf("解压路径已存在 %q", entry.Name)
		} else if !os.IsNotExist(e) {
			return e
		}
		input, e := entry.Open()
		if e != nil {
			return e
		}
		// OpenRoot confines traversal even if another process races path checks.
		output, e := root.OpenFile(filepath.FromSlash(name), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0644|entry.Mode().Perm()&0111)
		if e != nil {
			input.Close()
			return e
		}
		written, copyErr := io.Copy(output, io.LimitReader(contextReader{ctx, input}, int64(maxRuntimeFileBytes)+1))
		if copyErr == nil && uint64(written) > maxRuntimeFileBytes {
			copyErr = fmt.Errorf("压缩包展开大小超限")
		}
		closeErr := output.Close()
		inputErr := input.Close()
		if copyErr != nil {
			return copyErr
		}
		if closeErr != nil {
			return closeErr
		}
		if inputErr != nil {
			return inputErr
		}
		if err = ctx.Err(); err != nil {
			return err
		}
	}
	return nil
}

func runtimeEntryName(entry *zip.File) (string, error) {
	name := strings.ReplaceAll(entry.Name, "\\", "/")
	if name == "" || strings.HasPrefix(name, "/") || strings.ContainsRune(name, ':') || strings.ContainsRune(name, 0) || entry.Mode()&os.ModeSymlink != 0 || (!entry.FileInfo().IsDir() && !entry.Mode().IsRegular()) {
		return "", fmt.Errorf("压缩包包含不安全的路径 %q", entry.Name)
	}
	name = strings.TrimSuffix(name, "/")
	for _, part := range strings.Split(name, "/") {
		if part == "" || part == "." || part == ".." || strings.TrimRight(part, " .") != part {
			return "", fmt.Errorf("压缩包包含不安全的路径 %q", entry.Name)
		}
		for _, ch := range part {
			if ch < 32 || strings.ContainsRune(`<>"|?*`, ch) {
				return "", fmt.Errorf("压缩包包含不安全的路径 %q", entry.Name)
			}
		}
		base := strings.ToUpper(strings.TrimRight(strings.SplitN(part, ".", 2)[0], " "))
		if base == "CON" || base == "PRN" || base == "AUX" || base == "NUL" || base == "CONIN$" || base == "CONOUT$" || (len([]rune(base)) == 4 && (strings.HasPrefix(base, "COM") || strings.HasPrefix(base, "LPT")) && strings.ContainsRune("123456789¹²³", []rune(base)[3])) {
			return "", fmt.Errorf("压缩包包含设备路径 %q", entry.Name)
		}
	}
	return name, nil
}

// Check every existing ancestor; MkdirAll alone follows symlinks, including a
// symlink supplied as destination itself. Archive names cannot create symlinks.
func safeDirectory(directory string) error {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return err
	}
	parent := filepath.Dir(absolute)
	if parent != absolute {
		if err = safeDirectory(parent); err != nil {
			return err
		}
	}
	info, err := os.Lstat(absolute)
	if os.IsNotExist(err) {
		return os.Mkdir(absolute, 0755)
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("解压目录包含链接或非目录 %q", absolute)
	}
	return nil
}

type contextReader struct {
	ctx    context.Context
	source io.Reader
}

func (r contextReader) Read(p []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.Read(p)
}

type contextReaderAt struct {
	ctx    context.Context
	source io.ReaderAt
}

func (r contextReaderAt) ReadAt(p []byte, off int64) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.source.ReadAt(p, off)
}
