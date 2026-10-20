package api

import (
	"net/http"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

type Server struct{ Services *svc.ServiceContext }
type identity struct {
	account     model.User
	sessionHash string
}
type authHandler func(http.ResponseWriter, *http.Request, identity)

func (server *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", server.health)
	mux.HandleFunc("POST /api/v1/auth/register", server.register)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("POST /api/v1/auth/logout", server.auth(server.logout))
	mux.HandleFunc("PATCH /api/v1/me", server.auth(server.updateMe))
	mux.HandleFunc("GET /api/v1/me", server.auth(func(writer http.ResponseWriter, request *http.Request, caller identity) {
		respond(writer, 200, caller.account)
	}))
	mux.HandleFunc("GET /api/v1/users", server.auth(server.users))
	mux.HandleFunc("GET /api/v1/friends", server.auth(server.friends))
	mux.HandleFunc("POST /api/v1/friends", server.auth(server.addFriend))
	mux.HandleFunc("GET /api/v1/friends/requests", server.auth(server.friendRequests))
	mux.HandleFunc("POST /api/v1/friends/requests/{id}/{action}", server.auth(server.decideFriendRequest))
	mux.HandleFunc("GET /api/v1/conversations", server.auth(server.conversations))
	mux.HandleFunc("POST /api/v1/conversations", server.auth(server.create))
	mux.HandleFunc("POST /api/v1/conversations/{id}/members", server.auth(server.addMembers))
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/members", server.auth(server.removeMember))
	mux.HandleFunc("POST /api/v1/conversations/{id}/members/leave", server.auth(server.leaveGroup))
	mux.HandleFunc("POST /api/v1/conversations/{id}/owner", server.auth(server.transferOwnership))
	mux.HandleFunc("POST /api/v1/conversations/{id}/roles", server.auth(server.setRole))
	mux.HandleFunc("PATCH /api/v1/conversations/{id}/announcement", server.auth(server.setAnnouncement))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", server.auth(server.history))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages/search", server.auth(server.searchMessages))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages/{seq}/delivery", server.auth(server.deliveryStatus))
	mux.HandleFunc("GET /api/v1/conversations/{id}/sync", server.auth(server.sync))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", server.auth(server.send))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages/{seq}/recall", server.auth(server.recall))
	mux.HandleFunc("POST /api/v1/conversations/{id}/media", server.auth(server.uploadMedia))
	mux.HandleFunc("GET /api/v1/media/{media_id}", server.auth(server.downloadMedia))
	mux.HandleFunc("POST /api/v1/conversations/{id}/read", server.auth(server.read))
	mux.HandleFunc("POST /api/v1/conversations/{id}/received", server.auth(server.received))
	mux.HandleFunc("GET /api/v1/devices/{device_id}/cursors", server.auth(server.cursors))
	mux.HandleFunc("GET /ws", server.auth(server.websocket))
	return server.cors(mux)
}
