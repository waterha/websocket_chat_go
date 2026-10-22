package api

import (
	"context"
	"net/http"
	"strconv"
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

func (server *Server) directory(writer http.ResponseWriter, request *http.Request, caller identity) {
	params := request.URL.Query()
	offset, limit := 0, 50
	for key, target := range map[string]*int{"offset": &offset, "limit": &limit} {
		if params.Has(key) {
			value, err := strconv.Atoi(params.Get(key))
			if err != nil {
				fail(writer, model.Fail(400, "invalid_directory_query"))
				return
			}
			*target = value
		}
	}
	entries, err := server.Services.Auth.Directory(request.Context(), caller.account.ID, params.Get("q"), params.Get("department"), offset, limit)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, entries)
}

func (server *Server) workProfile(writer http.ResponseWriter, request *http.Request, caller identity) {
	profile, err := server.Services.Auth.WorkProfile(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, profile)
}

func (server *Server) updateWorkProfile(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.WorkProfile
	if !decode(writer, request, &input) {
		return
	}
	profile, err := server.Services.Auth.UpdateWorkProfile(request.Context(), caller.account.ID, input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, profile)
}

func (server *Server) addFriend(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.AddFriendInput
	if !decode(writer, request, &input) {
		return
	}
	friendRequest, err := server.Services.Auth.CreateFriendRequest(request.Context(), caller.account.ID, input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 201, friendRequest)
}

func (server *Server) friendRequests(writer http.ResponseWriter, request *http.Request, caller identity) {
	requests, err := server.Services.Auth.FriendRequests(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, requests)
}

func (server *Server) decideFriendRequest(writer http.ResponseWriter, request *http.Request, caller identity) {
	status := request.PathValue("action")
	if status == "accept" {
		status = "accepted"
	} else if status == "reject" {
		status = "rejected"
	}
	friendRequest, err := server.Services.Auth.DecideFriendRequest(request.Context(), caller.account.ID, request.PathValue("id"), status)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, friendRequest)
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
