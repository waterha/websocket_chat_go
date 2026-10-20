package realtime

import (
	"context"
	"errors"
	"sync"

	"websocket_chat_go/backend/internal/model"
)

var ErrClosed = errors.New("connection closed")

type Client struct {
	ID          string
	UserID      string
	DeviceID    string
	SessionHash string
	Outbound    chan model.Event
	Context     context.Context
	stop        context.CancelFunc
}

type Hub struct {
	mutex     sync.Mutex
	clients   map[string]map[string]*Client
	closed    bool
	queueSize int
}

func NewHub(queueSize int) *Hub {
	return &Hub{clients: make(map[string]map[string]*Client), queueSize: queueSize}
}

func (hub *Hub) Register(ctx context.Context, userID, deviceID, sessionHash string) (*Client, error) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	if hub.closed {
		return nil, ErrClosed
	}
	devices := hub.clients[userID]
	if devices == nil {
		devices = make(map[string]*Client)
		hub.clients[userID] = devices
	}
	if previous := devices[deviceID]; previous != nil {
		previous.stop()
	} else if len(devices) >= 8 {
		return nil, model.Fail(429, "too_many_devices")
	}
	clientContext, cancel := context.WithCancel(ctx)
	client := &Client{ID: model.NewID(), UserID: userID, DeviceID: deviceID, SessionHash: sessionHash, Outbound: make(chan model.Event, hub.queueSize), Context: clientContext, stop: cancel}
	devices[deviceID] = client
	return client, nil
}

func (hub *Hub) Unregister(client *Client) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	client.stop()
	devices := hub.clients[client.UserID]
	if devices[client.DeviceID] == client {
		delete(devices, client.DeviceID)
	}
	if len(devices) == 0 {
		delete(hub.clients, client.UserID)
	}
}

func (client *Client) Reply(event model.Event) bool {
	if client.Context.Err() != nil {
		return false
	}
	select {
	case client.Outbound <- event:
		return true
	default:
		client.stop()
		return false
	}
}

func (hub *Hub) Publish(recipients []string, event model.Event) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	for _, userID := range recipients {
		for _, client := range hub.clients[userID] {
			client.Reply(event)
		}
	}
}

func (hub *Hub) Clients() []*Client {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	clients := []*Client{}
	for _, devices := range hub.clients {
		for _, client := range devices {
			if client.Context.Err() == nil {
				clients = append(clients, client)
			}
		}
	}
	return clients
}

func (hub *Hub) DisconnectSession(sessionHash string) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	for _, devices := range hub.clients {
		for _, client := range devices {
			if client.SessionHash == sessionHash {
				client.stop()
			}
		}
	}
}

// DisconnectOthers closes every connection of a user except the session that
// must stay alive, so a password change ends all other logins immediately.
func (hub *Hub) DisconnectOthers(userID, keepSessionHash string) {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	for _, client := range hub.clients[userID] {
		if client.SessionHash != keepSessionHash {
			client.stop()
		}
	}
}

func (hub *Hub) Close() {
	hub.mutex.Lock()
	defer hub.mutex.Unlock()
	hub.closed = true
	for _, devices := range hub.clients {
		for _, client := range devices {
			client.stop()
		}
	}
}
