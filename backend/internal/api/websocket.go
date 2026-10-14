package api

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"time"

	"github.com/coder/websocket"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

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
