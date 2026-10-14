package api

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"time"

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
