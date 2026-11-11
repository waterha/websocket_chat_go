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

type socketDial struct {
	cancel context.CancelFunc
}

type SocketEvent struct {
	ID   string `json:"id"`
	Type string `json:"type"`
	Data string `json:"data,omitempty"`
}

func (c *Client) OpenSocket(id, device string, emit func(SocketEvent)) error {
	if !validID.MatchString(id) || !validID.MatchString(device) {
		return fmt.Errorf("无效的设备标识")
	}
	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()
	dial := &socketDial{cancel: cancel}
	c.mu.Lock()
	if c.ctx.Err() != nil || c.sockets[id] != nil || c.pending[id] != nil {
		c.mu.Unlock()
		return fmt.Errorf("连接已关闭或重复")
	}
	// Reserve before performing network I/O: duplicate opens do not consume a
	// second handshake and CloseSocket can cancel a stalled handshake at once.
	c.pending[id] = dial
	c.mu.Unlock()
	defer func() {
		c.mu.Lock()
		if c.pending[id] == dial {
			delete(c.pending, id)
		}
		c.mu.Unlock()
	}()
	target := *c.base
	if target.Scheme == "https" {
		target.Scheme = "wss"
	} else {
		target.Scheme = "ws"
	}
	target.Path = "/ws"
	target.RawQuery = url.Values{"device_id": []string{device}}.Encode()
	conn, resp, err := websocket.Dial(ctx, target.String(), &websocket.DialOptions{HTTPClient: c.http, HTTPHeader: http.Header{"Origin": []string{c.Config.Origin}}})
	if err != nil {
		if resp != nil && resp.Body != nil {
			resp.Body.Close()
		}
		return fmt.Errorf("实时连接失败：%w", err)
	}
	conn.SetReadLimit(4 << 20)
	c.mu.Lock()
	if ctx.Err() != nil || c.pending[id] != dial || c.sockets[id] != nil {
		c.mu.Unlock()
		conn.CloseNow()
		return fmt.Errorf("连接已关闭或重复")
	}
	delete(c.pending, id)
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
	dial := c.pending[id]
	if dial != nil {
		dial.cancel()
		delete(c.pending, id)
	}
	c.mu.Unlock()
	if conn != nil {
		// Abrupt close releases native resources immediately; reconnect/sync already
		// handles interrupted delivery and application shutdown.
		_ = conn.CloseNow()
	}
}
