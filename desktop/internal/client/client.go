// Package client owns desktop authentication and streaming network connections.
// Session cookies stay in the native cookie jar and never cross the JS bridge.
package client

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/coder/websocket"
)

type Client struct {
	Config  Config
	base    *url.URL
	http    *http.Client
	ctx     context.Context
	cancel  context.CancelFunc
	mu      sync.Mutex
	sockets map[string]*websocket.Conn
}

func New(config Config) (*Client, error) {
	c, err := config.Validate()
	if err != nil {
		return nil, err
	}
	base, _ := url.Parse(c.ServerURL)
	jar, _ := cookiejar.New(nil)
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{Config: c, base: base, ctx: ctx, cancel: cancel, sockets: make(map[string]*websocket.Conn), http: &http.Client{
		Jar: jar,
		Transport: &http.Transport{
			Proxy:               http.ProxyFromEnvironment,
			DialContext:         (&net.Dialer{Timeout: 5 * time.Second, KeepAlive: 30 * time.Second}).DialContext,
			TLSHandshakeTimeout: 10 * time.Second, ResponseHeaderTimeout: 15 * time.Second,
			IdleConnTimeout: 60 * time.Second, MaxIdleConns: 8, MaxIdleConnsPerHost: 4,
		},
		// A redirect must never forward an authenticated request to another host.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}}, nil
}

func (c *Client) request(ctx context.Context, method, path string, body io.Reader, headers http.Header) (*http.Response, error) {
	relative, err := url.Parse(path)
	if err != nil || relative.IsAbs() || relative.Host != "" || (!strings.HasPrefix(relative.Path, "/api/v1/") && relative.Path != "/healthz") {
		return nil, fmt.Errorf("无效的请求地址")
	}
	target := c.base.ResolveReference(relative)
	req, err := http.NewRequestWithContext(ctx, method, target.String(), body)
	if err != nil {
		return nil, err
	}
	if headers != nil {
		req.Header = headers.Clone()
		if length := headers.Get("Content-Length"); length != "" {
			req.ContentLength, err = strconv.ParseInt(length, 10, 64)
			if err != nil || req.ContentLength < 0 {
				return nil, fmt.Errorf("无效的附件大小")
			}
		}
	}
	req.Header.Del("Cookie")
	req.Header.Del("X-CSRF-Token")
	req.Header.Set("Origin", c.Config.Origin)
	for _, cookie := range c.http.Jar.Cookies(target) {
		if cookie.Name == "csrf" {
			req.Header.Set("X-CSRF-Token", cookie.Value)
		}
	}
	return c.http.Do(req)
}

func (c *Client) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 10*time.Minute)
	defer cancel()
	stop := context.AfterFunc(c.ctx, cancel)
	defer stop()
	headers := make(http.Header)
	if r.ContentLength > 0 {
		headers.Set("Content-Length", strconv.FormatInt(r.ContentLength, 10))
	} else if length := r.Header.Get("X-Upload-Size"); length != "" {
		// WebView resource interception may omit ContentLength. The frontend
		// supplies File.size without copying the file into a bridge payload.
		headers.Set("Content-Length", length)
	}
	for _, key := range []string{"Content-Type", "Range", "If-Range", "If-None-Match", "If-Modified-Since"} {
		if value := r.Header.Get(key); value != "" {
			headers.Set(key, value)
		}
	}
	resp, err := c.request(ctx, r.Method, r.URL.RequestURI(), r.Body, headers)
	if err != nil {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadGateway)
		_ = json.NewEncoder(w).Encode(map[string]any{"error": map[string]string{"code": "desktop_connection_failed", "message": "无法连接服务器，请检查服务是否启动及服务器地址"}})
		return
	}
	defer resp.Body.Close()
	for _, key := range []string{"Content-Type", "Content-Length", "Content-Disposition", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified", "X-Content-Type-Options"} {
		if value := resp.Header.Get(key); value != "" {
			w.Header().Set(key, value)
		}
	}
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(resp.StatusCode)
	_, _ = io.Copy(w, resp.Body)
}

func (c *Client) Check(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	resp, err := c.request(ctx, http.MethodGet, "/healthz", nil, nil)
	if err != nil {
		return fmt.Errorf("无法连接服务器，请检查地址及网络：%w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("服务器尚未就绪（HTTP %d）", resp.StatusCode)
	}
	return nil
}

func (c *Client) Close() {
	c.cancel()
	c.http.CloseIdleConnections()
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, socket := range c.sockets {
		_ = socket.CloseNow()
	}
}
