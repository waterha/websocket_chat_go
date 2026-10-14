package main

import (
	"encoding/json"
	"github.com/coder/websocket"
	"net/http"
	"sync"
	"sync/atomic"
	"websocket_chat_go/backend/internal/model"
)

type envelope struct {
	Event     string          `json:"event"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
	Error     *model.Error    `json:"error"`
}

type client struct {
	account   credentials
	fixture   *fixture
	http      *http.Client
	stats     *statistics
	mutex     sync.Mutex
	socket    *websocket.Conn
	pending   map[string]chan envelope
	notify    chan struct{}
	ready     chan struct{}
	received  atomic.Int64
	confirmed atomic.Int64
	target    atomic.Int64
}
