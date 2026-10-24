package api

import (
	"context"
	"net/http"
	"strconv"
	"strings"
	"time"

	"websocket_chat_go/backend/internal/logic"
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

// setFriendRemark updates the caller's private note for one friend.
func (server *Server) setFriendRemark(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.FriendRemarkInput
	if !decode(writer, request, &input) {
		return
	}
	friend, err := server.Services.Auth.SetFriendRemark(request.Context(), caller.account.ID, request.PathValue("friend_id"), input.Remark)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, friend)
}

// deleteFriend ends a friendship in both directions; shared conversations stay.
func (server *Server) deleteFriend(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Auth.DeleteFriend(request.Context(), caller.account.ID, request.PathValue("friend_id")); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("friend_id")})
}

// blocks lists the caller's blocked users.
func (server *Server) blocks(writer http.ResponseWriter, request *http.Request, caller identity) {
	blocks, err := server.Services.Auth.Blocks(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, blocks)
}

// block adds a user to the caller's block list.
func (server *Server) block(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.BlockInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Auth.Block(request.Context(), caller.account.ID, input.UserID); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 201, map[string]string{"id": input.UserID})
}

// unblock removes a user from the caller's block list.
func (server *Server) unblock(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Auth.Unblock(request.Context(), caller.account.ID, request.PathValue("user_id")); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("user_id")})
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

// setAvatar stores an uploaded image as the caller's avatar. The body is the
// raw image, exactly like a conversation media upload.
func (server *Server) setAvatar(writer http.ResponseWriter, request *http.Request, caller identity) {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]))
	kind, ok := logic.MediaKind(contentType)
	if !ok || kind != "image" {
		fail(writer, model.Fail(415, "unsupported_media_type"))
		return
	}
	size := request.ContentLength
	if size <= 0 {
		fail(writer, model.Fail(400, "invalid_upload"))
		return
	}
	if size > logic.MaxAvatarBytes {
		fail(writer, model.Fail(413, "avatar_too_large"))
		return
	}
	fileName := request.URL.Query().Get("file_name")
	if fileName == "" {
		fileName = "avatar" + extensionFor(contentType)
	}
	request.Body = http.MaxBytesReader(writer, request.Body, logic.MaxAvatarBytes+(1<<20))
	account, err := server.Services.Media.UploadAvatar(request.Context(), caller.account.ID, fileName, contentType, size, request.Body)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, account)
}

// clearAvatar removes the caller's avatar reference.
func (server *Server) clearAvatar(writer http.ResponseWriter, request *http.Request, caller identity) {
	account, err := server.Services.Media.ClearAvatar(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, account)
}
