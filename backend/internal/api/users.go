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

func (server *Server) friends(writer http.ResponseWriter, request *http.Request, caller identity) {
	friends, err := server.Services.Auth.Friends(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, friends)
}

func (server *Server) addFriend(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.AddFriendInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Auth.AddFriend(request.Context(), caller.account.ID, input); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"user_id": input.UserID})
}

func (server *Server) updateMe(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.ProfileInput
	if !decode(writer, request, &input) {
		return
	}
	account, err := server.Services.Auth.UpdateProfile(request.Context(), caller.account.ID, input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, account)
}
