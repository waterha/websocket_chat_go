package api

import (
	"context"
	"net/http"
	"time"
	"websocket_chat_go/backend/internal/model"
)

func (server *Server) health(writer http.ResponseWriter, request *http.Request) {
	ctx, cancel := context.WithTimeout(request.Context(), 2*time.Second)
	defer cancel()
	if server.Services.Store.DB.Ping(ctx) != nil || server.Services.Redis.Ping(ctx).Err() != nil {
		fail(writer, model.Fail(503, "dependency_unavailable"))
		return
	}
	respond(writer, 200, map[string]string{"status": "ok"})
}

func (server *Server) users(writer http.ResponseWriter, request *http.Request, caller identity) {
	users, err := server.Services.Auth.Users(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, users)
}
