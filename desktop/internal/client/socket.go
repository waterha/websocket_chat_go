package client

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"time"

	"github.com/coder/websocket"
)

var validID = regexp.MustCompile(`^[A-Za-z0-9_-]{1,128}$`)

type SocketEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

func (c *Client) OpenSocket(id, device string, emit func(SocketEvent)) error {
	if !validID.MatchString(id) || !validID.MatchString(device) {
		return fmt.Errorf("无效的设备标识")
	}
	target := *c.base
	if target.Scheme == "https" {
		target.Scheme = "wss"
	} else {
		target.Scheme = "ws"
	}
	target.Path = "/ws"
	target.RawQuery = url.Values{"device_id": []string{device}}.Encode()
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	conn, resp, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: http.Header{"Origin": []string{c.Config.Origin}}})
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return fmt.Errorf("实时连接失败：%w", err)
	}
	conn.SetReadLimit(4 << 20)
	c.mu.Lock()
	if c.ctx.Err() != nil || c.sockets[id] != nil {
		c.mu.Unlock()
		conn.CloseNow()
		return fmt.Errorf("连接已关闭或重复")
	}
	c.sockets[id] = conn
	c.mu.Unlock()
	go func() {
		defer func() {
			conn.CloseNow()
			c.mu.Lock()
			delete(c.sockets, id)
			c.mu.Unlock()
			emit(SocketEvent{ID: id, Type: "close"})
		}()
		emit(SocketEvent{ID: id, Type: "open"})
		for {
			kind, data, err := conn.Read(c.ctx)
			if err != nil {
				return
			}
			if kind == websocket.MessageText {
				emit(SocketEvent{ID: id, Type: "message", Data: string(data)})
			}
		}
	}()
	return nil
}

func (c *Client) SendSocket(id, payload string) error {
	if len(payload) > 64<<10 {
		return fmt.Errorf("消息过大")
	}
	c.mu.Lock()
	conn := c.sockets[id]
	c.mu.Unlock()
	if conn == nil {
		return fmt.Errorf("实时连接未就绪")
	}
	ctx, cancel := context.WithTimeout(c.ctx, 8*time.Second)
	defer cancel()
	return conn.Write(ctx, websocket.MessageText, []byte(payload))
}

func (c *Client) CloseSocket(id string) {
	c.mu.Lock()
	conn := c.sockets[id]
	c.mu.Unlock()
	if conn != nil {
		// Abrupt close releases native resources immediately; reconnect/sync already
		// handles interrupted delivery and application shutdown.
		_ = conn.CloseNow()
	}
}
