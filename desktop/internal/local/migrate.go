package local

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
)

// extractMigrations writes the migrations embedded in the launcher into a real
// directory for the chat server, which reads them by path. The directory is
// rebuilt on every start so an updated launcher always ships current schema.
func extractMigrations(source fs.FS, destination string) error {
	if source == nil {
		return fmt.Errorf("启动器未包含数据库迁移文件，请使用 scripts\\build-desktop.ps1 构建")
	}
	entries, err := fs.Glob(source, "*.up.sql")
	if err != nil {
		return err
	}
	if len(entries) == 0 {
		return fmt.Errorf("启动器未包含数据库迁移文件，请使用 scripts\\build-desktop.ps1 构建")
	}
	if err := os.RemoveAll(destination); err != nil {
		return err
	}
	if err := os.MkdirAll(destination, 0755); err != nil {
		return err
	}
	return os.CopyFS(destination, source)
}

func mediaDirectory(root string) string { return filepath.Join(root, "media") }
