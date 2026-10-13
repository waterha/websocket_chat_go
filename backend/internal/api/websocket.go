package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

type command struct {
	Event     string          `json:"event"`
	RequestID string          `json:"request_id"`
	Data      json.RawMessage `json:"data"`
}

func parseJSON(payload []byte, target any) error {
	decoder := json.NewDecoder(bytes.NewReader(payload))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return model.Fail(400, "invalid_event")
	}
	var extra any
	if err := decoder.Decode(&extra); !errors.Is(err, io.EOF) {
		return model.Fail(400, "invalid_event")
	}
	return nil
}

func (server *Server) websocket(writer http.ResponseWriter, request *http.Request, caller identity) {
	if request.Header.Get("Origin") != server.Services.Config.Origin {
		fail(writer, model.Fail(403, "invalid_origin"))
		return
	}
	deviceID := request.URL.Query().Get("device_id")
	if !logic.ValidDeviceID(deviceID) {
		fail(writer, model.Fail(400, "invalid_device"))
		return
	}
	origin, err := url.Parse(server.Services.Config.Origin)
	if err != nil {
		fail(writer, err)
		return
	}
	connection, err := websocket.Accept(writer, request, &websocket.AcceptOptions{OriginPatterns: []string{origin.Host}})
	if err != nil {
		return
	}
	defer connection.CloseNow()
	client, err := server.Services.Hub.Register(request.Context(), caller.account.ID, deviceID, caller.sessionHash)
	if err != nil {
		_ = connection.Close(websocket.StatusPolicyViolation, err.Error())
		return
	}
	defer server.Services.Hub.Unregister(client)
	connection.SetReadLimit(64 << 10)
	ready, err := json.Marshal(model.Event{Event: "connection.ready", Data: map[string]any{"device_id": deviceID, "connection_id": client.ID, "heartbeat_seconds": server.Services.Config.Heartbeat.Seconds()}})
	if err != nil {
		return
	}
	readyContext, cancel := context.WithTimeout(client.Context, 5*time.Second)
	err = connection.Write(readyContext, websocket.MessageText, ready)
	cancel()
	if err != nil {
		return
	}
	writerDone := make(chan struct{})
	go func() { defer close(writerDone); server.writeLoop(connection, client) }()
	for {
		kind, payload, err := connection.Read(client.Context)
		if err != nil {
			break
		}
		if kind != websocket.MessageText {
			client.Reply(model.Event{Event: "error", Error: publicError(model.Fail(400, "text_events_only"))})
			continue
		}
		var input command
		if err := parseJSON(payload, &input); err != nil {
			client.Reply(model.Event{Event: "error", Error: publicError(err)})
			continue
		}
		if len(input.RequestID) < 1 || len(input.RequestID) > 100 {
			client.Reply(model.Event{Event: "error", Error: publicError(model.Fail(400, "invalid_request_id"))})
			continue
		}
		ctx, cancel := context.WithTimeout(client.Context, 10*time.Second)
		event, err := server.handleCommand(ctx, client, input)
		cancel()
		if err != nil {
			event = model.Event{Event: "error", Error: publicError(err)}
		}
		event.RequestID = input.RequestID
		if !client.Reply(event) {
			break
		}
	}
	server.Services.Hub.Unregister(client)
	<-writerDone
}

func (server *Server) handleCommand(ctx context.Context, client *realtime.Client, input command) (model.Event, error) {
	switch input.Event {
	case "message.send":
		var body model.SendInput
		if err := parseJSON(input.Data, &body); err != nil {
			return model.Event{}, err
		}
		message, _, err := server.Services.Chat.Send(ctx, client.UserID, body)
		return model.Event{Event: "message.ack", Data: message}, err
	case "conversation.read":
		var body struct {
			ConversationID string `json:"conversation_id"`
			Seq            int64  `json:"seq"`
		}
		if err := parseJSON(input.Data, &body); err != nil {
			return model.Event{}, err
		}
		receipt, err := server.Services.Chat.MarkRead(ctx, client.UserID, body.ConversationID, body.Seq)
		return model.Event{Event: "conversation.read.ack", Data: receipt}, err
	case "message.received":
		var body struct {
			ConversationID string `json:"conversation_id"`
			Seq            int64  `json:"seq"`
		}
		if err := parseJSON(input.Data, &body); err != nil {
			return model.Event{}, err
		}
		cursor, err := server.Services.Chat.MarkReceived(ctx, client.UserID, body.ConversationID, client.DeviceID, body.Seq)
		return model.Event{Event: "message.received.ack", Data: cursor}, err
	case "ping":
		return model.Event{Event: "pong", Data: map[string]any{"server_time": time.Now().UTC()}}, nil
	default:
		return model.Event{}, model.Fail(400, "unknown_event")
	}
}

func (server *Server) presence(ctx context.Context, client *realtime.Client) {
	key := "im:presence:" + client.UserID + ":" + client.DeviceID + ":" + client.ID
	if err := server.Services.Redis.Set(ctx, key, client.ID, 3*server.Services.Config.Heartbeat).Err(); err != nil {
		log.Printf("presence refresh: %v", err)
	}
}

func (server *Server) writeLoop(connection *websocket.Conn, client *realtime.Client) {
	defer connection.CloseNow()
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = server.Services.Redis.Del(ctx, "im:presence:"+client.UserID+":"+client.DeviceID+":"+client.ID).Err()
	}()
	server.presence(client.Context, client)
	timer := time.NewTicker(server.Services.Config.Heartbeat)
	defer timer.Stop()
	for {
		select {
		case <-client.Context.Done():
			return
		case event := <-client.Outbound:
			ctx, cancel := context.WithTimeout(client.Context, 5*time.Second)
			payload, err := json.Marshal(event)
			if err == nil {
				err = connection.Write(ctx, websocket.MessageText, payload)
			}
			cancel()
			if err != nil {
				return
			}
		case <-timer.C:
			ctx, cancel := context.WithTimeout(client.Context, server.Services.Config.HeartbeatTimeout)
			_, err := server.Services.Auth.Authenticate(ctx, client.SessionHash)
			if err == nil {
				err = connection.Ping(ctx)
			}
			if err == nil {
				server.presence(ctx, client)
			}
			cancel()
			if err != nil {
				return
			}
		}
	}
}
