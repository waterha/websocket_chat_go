package local

import (
	"context"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"
)

type redisService struct {
	bin  string
	data string
	port int

	startMu sync.Mutex
	started bool
	process *os.Process
}

func (service *redisService) address() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(service.port))
}

// redisPing speaks the two commands of RESP the launcher needs, avoiding a
// dependency on redis-cli.exe.
func redisPing(address string, timeout time.Duration) bool {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return false
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	if _, err := connection.Write([]byte("*1\r\n$4\r\nPING\r\n")); err != nil {
		return false
	}
	buffer := make([]byte, 32)
	read, err := connection.Read(buffer)
	return err == nil && string(buffer[:min(read, 5)]) == "+PONG"
}

// redisShutdown asks the server to exit cleanly; the connection drops without
// a reply, so only the following port check confirms the outcome.
func redisShutdown(address string, timeout time.Duration) bool {
	connection, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return true
	}
	defer connection.Close()
	_ = connection.SetDeadline(time.Now().Add(timeout))
	_, _ = connection.Write([]byte("*2\r\n$8\r\nSHUTDOWN\r\n$6\r\nNOSAVE\r\n"))
	for start := time.Now(); time.Since(start) < 10*time.Second; {
		if !tcpOpen(portOf(address)) {
			return true
		}
		time.Sleep(200 * time.Millisecond)
	}
	return false
}

func portOf(address string) int {
	_, port, err := net.SplitHostPort(address)
	if err != nil {
		return 0
	}
	value, _ := strconv.Atoi(port)
	return value
}

// ensure starts Redis unless an instance already answers on the port.
func (service *redisService) ensure(ctx context.Context, progress func(string)) error {
	address := service.address()
	if redisPing(address, time.Second) {
		progress(fmt.Sprintf("检测到 Redis 已在运行（端口 %d）", service.port))
		return nil
	}
	if service.bin == "" {
		return fmt.Errorf("未找到 Redis，且自动下载失败。可手动解压便携版 Redis 到 %s，或设置环境变量 IM_REDIS_DIR 指向其目录", filepath.Join(filepath.Dir(service.data), "redis"))
	}
	if err := os.MkdirAll(service.data, 0755); err != nil {
		return err
	}
	progress(fmt.Sprintf("启动 Redis（端口 %d）", service.port))
	logFile, err := os.OpenFile(filepath.Join(service.data, "redis.log"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer logFile.Close()
	command := exec.CommandContext(ctx, filepath.Join(service.bin, exeName("redis-server")),
		"--port", strconv.Itoa(service.port), "--bind", "127.0.0.1", "--dir", service.data,
		"--appendonly", "no", "--save", "")
	command.Stdout, command.Stderr = logFile, logFile
	hideWindow(command)
	if err := command.Start(); err != nil {
		return fmt.Errorf("Redis 启动失败：%w", err)
	}
	service.startMu.Lock()
	service.started = true
	service.process = command.Process
	service.startMu.Unlock()
	for start := time.Now(); !redisPing(address, time.Second); {
		if time.Since(start) > 20*time.Second {
			return fmt.Errorf("Redis 启动后未响应端口 %d", service.port)
		}
		select {
		case <-ctx.Done():
			return fmt.Errorf("等待 Redis 启动时被取消")
		case <-time.After(300 * time.Millisecond):
		}
	}
	return nil
}

func (service *redisService) stop() {
	service.startMu.Lock()
	if !service.started {
		service.startMu.Unlock()
		return
	}
	service.started = false
	process := service.process
	service.process = nil
	service.startMu.Unlock()
	if process == nil {
		return
	}
	if !redisShutdown(service.address(), time.Second) {
		_ = process.Kill()
	}
}
