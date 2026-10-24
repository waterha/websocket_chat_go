// Command bootstrap-check exercises the launcher's service stack headlessly:
// it runs the same Manager the desktop app uses against real PostgreSQL and
// Redis, then verifies the chat server answers and shuts everything down.
// It is a manual verification tool, not part of the build. Example:
//
//	set BOOTSTRAP_CHECK_MIGRATIONS=..\database\migrations
//	set IM_REDIS_DIR=C:\path\to\portable-redis
//	set IM_DESKTOP_PGPORT=15434
//	set IM_DESKTOP_REDIS_PORT=16380
//	go run ./cmd/bootstrap-check
package main

import (
	"context"
	"fmt"
	"os"
	"time"

	"websocket_chat_go/desktop/internal/client"
	"websocket_chat_go/desktop/internal/local"
)

func main() {
	root := os.Getenv("BOOTSTRAP_CHECK_ROOT")
	if root == "" {
		root, _ = os.MkdirTemp("", "bootstrap-check-*")
		defer os.RemoveAll(root)
	}
	config := client.Config{ServerURL: "http://127.0.0.1:18080", Origin: "http://127.0.0.1:4173"}
	manager := local.New(root, os.DirFS(os.Getenv("BOOTSTRAP_CHECK_MIGRATIONS")))
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := time.Now()
	err := manager.Run(ctx, config, func(message string) {
		fmt.Printf("[%4.1fs] %s\n", time.Since(started).Seconds(), message)
	})
	if err != nil {
		fmt.Println("失败：", err)
		os.Exit(1)
	}
	fmt.Printf("[%4.1fs] 健康检查通过：%s/healthz\n", time.Since(started).Seconds(), config.ServerURL)
	fmt.Println("正在停止由启动器启动的服务…")
	manager.Stop(15 * time.Second)
	fmt.Println("已全部停止。")
}
