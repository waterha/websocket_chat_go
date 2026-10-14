package main

import (
	"context"
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"net/http"
	"strings"
	"time"
	"websocket_chat_go/backend/internal/model"
)

func (peer *client) signal(seq int64) {
	for previous := peer.target.Load(); seq > previous; previous = peer.target.Load() {
		if peer.target.CompareAndSwap(previous, seq) {
			break
		}
	}
	select {
	case peer.notify <- struct{}{}:
	default:
	}
}

func (peer *client) connect(ctx context.Context, gate chan struct{}) {
	first := true
	for ctx.Err() == nil {
		select {
		case gate <- struct{}{}:
		case <-ctx.Done():
			return
		}
		dialContext, cancel := context.WithTimeout(ctx, 15*time.Second)
		connection, response, err := websocket.Dial(dialContext, "ws"+strings.TrimPrefix(peer.fixture.URL, "http")+"/ws?device_id=load", &websocket.DialOptions{HTTPClient: peer.http, HTTPHeader: http.Header{"Origin": {peer.fixture.Origin}, "Cookie": {"session=" + peer.account.Token}}})
		if response != nil && err != nil {
			_ = response.Body.Close()
		}
		if err == nil {
			_, payload, readErr := connection.Read(dialContext)
			var event envelope
			err = readErr
			if err == nil {
				err = json.Unmarshal(payload, &event)
			}
			if err == nil && event.Event != "connection.ready" {
				err = fmt.Errorf("first frame was %q", event.Event)
			}
		}
		cancel()
		<-gate
		if err != nil {
			if connection != nil {
				_ = connection.CloseNow()
			}
			if ctx.Err() == nil {
				peer.stats.dialErrors.Add(1)
				peer.stats.failure("dial", err)
			}
			pause(ctx, time.Second)
			continue
		}
		peer.mutex.Lock()
		peer.socket = connection
		peer.mutex.Unlock()
		peer.stats.connections.Add(1)
		if first {
			close(peer.ready)
			first = false
		} else {
			peer.stats.reconnects.Add(1)
		}
		peer.signal(peer.target.Load())
		peer.read(ctx, connection)
		peer.mutex.Lock()
		peer.socket = nil
		for id, waiter := range peer.pending {
			waiter <- envelope{Error: &model.Error{Code: "connection_closed"}}
			delete(peer.pending, id)
		}
		peer.mutex.Unlock()
		_ = connection.CloseNow()
		peer.stats.connections.Add(-1)
		if ctx.Err() == nil {
			peer.stats.disconnects.Add(1)
			pause(ctx, time.Second)
		}
	}
}

func (peer *client) read(ctx context.Context, connection *websocket.Conn) {
	for {
		_, payload, err := connection.Read(ctx)
		if err != nil {
			return
		}
		var event envelope
		if err = json.Unmarshal(payload, &event); err != nil {
			peer.stats.failure("protocol", err)
			continue
		}
		if event.RequestID != "" {
			peer.mutex.Lock()
			if waiter := peer.pending[event.RequestID]; waiter != nil {
				waiter <- event
				delete(peer.pending, event.RequestID)
			}
			peer.mutex.Unlock()
			continue
		}
		switch event.Event {
		case "message.created":
			var message model.Message
			if err := json.Unmarshal(event.Data, &message); err != nil || message.ConversationID != peer.fixture.Group {
				peer.stats.failure("protocol", fmt.Errorf("invalid message push"))
				continue
			}
			peer.stats.pushes.Add(1)
			peer.stats.pushed(message.ClientMsgID)
			peer.signal(message.Seq)
		case "message.sync_required":
			var body struct {
				Cursors []model.DeviceCursor `json:"cursors"`
			}
			if err := json.Unmarshal(event.Data, &body); err != nil {
				peer.stats.failure("protocol", err)
				continue
			}
			peer.stats.compensations.Add(1)
			for _, cursor := range body.Cursors {
				peer.signal(cursor.LastSeq)
			}
		}
	}
}

func (peer *client) send(ctx context.Context, input model.SendInput) error {
	waiter := make(chan envelope, 1)
	peer.mutex.Lock()
	connection := peer.socket
	if connection != nil {
		peer.pending[input.ClientMsgID] = waiter
	}
	peer.mutex.Unlock()
	if connection == nil {
		return fmt.Errorf("sender offline")
	}
	defer func() {
		peer.mutex.Lock()
		delete(peer.pending, input.ClientMsgID)
		peer.mutex.Unlock()
	}()
	request, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	payload, _ := json.Marshal(model.Event{Event: "message.send", RequestID: input.ClientMsgID, Data: input})
	if err := connection.Write(request, websocket.MessageText, payload); err != nil {
		return err
	}
	select {
	case event := <-waiter:
		if event.Error != nil {
			return event.Error
		}
		var message model.Message
		if err := json.Unmarshal(event.Data, &message); err != nil || event.Event != "message.ack" || message.ClientMsgID != input.ClientMsgID {
			return fmt.Errorf("invalid send acknowledgement")
		}
		return nil
	case <-request.Done():
		return request.Err()
	}
}
