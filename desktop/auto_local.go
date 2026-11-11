package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"
	"websocket_chat_go/desktop/internal/client"
	"websocket_chat_go/desktop/internal/local"
)

// Only an unconfigured first launch may choose a different local HTTP port.
// Explicit server settings, including remote team servers, are never replaced.
func automaticLocalConfig(path string, config client.Config) (client.Config, error) {
	if os.Getenv("IM_DESKTOP_SERVER") != "" {
		return config, nil
	}
	if _, err := os.Stat(path); err == nil {
		return config, nil
	} else if !os.IsNotExist(err) {
		return config, err
	}
	target, ok := local.Plan(config)
	if !ok {
		return config, nil
	}
	connection, err := net.DialTimeout("tcp", target.Address(), 300*time.Millisecond)
	if err != nil {
		return config, nil
	}
	connection.Close()
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	request, _ := http.NewRequestWithContext(ctx, http.MethodGet, config.ServerURL+"/healthz", nil)
	check := &http.Client{Timeout: time.Second, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	response, healthErr := check.Do(request)
	if healthErr == nil {
		response.Body.Close()
		if response.StatusCode == http.StatusOK {
			return config, nil
		}
	}
	for port := 18081; port < 18101; port++ {
		listener, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
		if err != nil {
			continue
		}
		listener.Close()
		config.ServerURL = fmt.Sprintf("http://127.0.0.1:%d", port)
		if err := client.Save(path, config); err != nil {
			return config, err
		}
		return config, nil
	}
	return config, fmt.Errorf("无法为内置聊天服务分配本机端口")
}
