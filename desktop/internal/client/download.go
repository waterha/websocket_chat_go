package client

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"time"
)

// Download streams into a temporary file beside the destination. Failed and
// cancelled transfers leave an existing destination intact and no partial file.
func (c *Client) Download(ctx context.Context, id, destination string) error {
	if !validID.MatchString(id) {
		return fmt.Errorf("无效的附件标识")
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Minute)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	resp, err := c.request(ctx, http.MethodGet, "/api/v1/media/"+id, nil, nil)
	if err != nil {
		return fmt.Errorf("下载失败：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("附件不可下载（HTTP %d），请检查登录状态或消息是否已撤回", resp.StatusCode)
	}
	file, err := os.CreateTemp(filepath.Dir(destination), ".im-download-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	_, copyErr := io.Copy(file, resp.Body)
	closeErr := file.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	return os.Rename(file.Name(), destination)
}
