package api

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/coder/websocket"
	"websocket_chat_go/backend/internal/realtime"
)

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
