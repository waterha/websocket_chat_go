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
	// Login and registration are unauthenticated and therefore cannot carry a
	// session CSRF token, but they are still protected by SameSite cookies and
	// the login limiter.
	mux.HandleFunc("POST /api/v1/auth/register", server.register)
	mux.HandleFunc("POST /api/v1/auth/login", server.login)
	mux.HandleFunc("POST /api/v1/auth/logout", server.auth(server.logout))
	mux.HandleFunc("POST /api/v1/auth/password", server.auth(server.changePassword))
	mux.HandleFunc("PATCH /api/v1/me", server.auth(server.updateMe))
	mux.HandleFunc("POST /api/v1/me/avatar", server.auth(server.setAvatar))
	mux.HandleFunc("DELETE /api/v1/me/avatar", server.auth(server.clearAvatar))
	mux.HandleFunc("GET /api/v1/me", server.auth(func(writer http.ResponseWriter, request *http.Request, caller identity) {
		respond(writer, 200, caller.account)
	}))
	mux.HandleFunc("GET /api/v1/users", server.auth(server.users))
	mux.HandleFunc("GET /api/v1/directory", server.auth(server.directory))
	mux.HandleFunc("GET /api/v1/directory/me", server.auth(server.workProfile))
	mux.HandleFunc("PATCH /api/v1/directory/me", server.auth(server.updateWorkProfile))
	mux.HandleFunc("GET /api/v1/friends", server.auth(server.friends))
	mux.HandleFunc("POST /api/v1/friends", server.auth(server.addFriend))
	mux.HandleFunc("PATCH /api/v1/friends/{friend_id}", server.auth(server.setFriendRemark))
	mux.HandleFunc("DELETE /api/v1/friends/{friend_id}", server.auth(server.deleteFriend))
	mux.HandleFunc("GET /api/v1/friends/requests", server.auth(server.friendRequests))
	mux.HandleFunc("POST /api/v1/friends/requests/{id}/{action}", server.auth(server.decideFriendRequest))
	mux.HandleFunc("GET /api/v1/blocks", server.auth(server.blocks))
	mux.HandleFunc("POST /api/v1/blocks", server.auth(server.block))
	mux.HandleFunc("DELETE /api/v1/blocks/{user_id}", server.auth(server.unblock))
	mux.HandleFunc("GET /api/v1/messages/search", server.auth(server.searchAll))
	mux.HandleFunc("GET /api/v1/conversations", server.auth(server.conversations))
	mux.HandleFunc("GET /api/v1/groups/discover", server.auth(server.discoverGroups))
	mux.HandleFunc("GET /api/v1/groups/join-requests", server.auth(server.myJoinRequests))
	mux.HandleFunc("POST /api/v1/groups/{id}/join-requests", server.auth(server.requestToJoin))
	mux.HandleFunc("DELETE /api/v1/groups/{id}/join-requests", server.auth(server.cancelJoinRequest))
	mux.HandleFunc("POST /api/v1/conversations", server.auth(server.create))
	mux.HandleFunc("DELETE /api/v1/conversations/{id}", server.auth(server.deleteConversation))
	mux.HandleFunc("POST /api/v1/conversations/{id}/members", server.auth(server.addMembers))
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/members", server.auth(server.removeMember))
	mux.HandleFunc("POST /api/v1/conversations/{id}/members/leave", server.auth(server.leaveGroup))
	mux.HandleFunc("PATCH /api/v1/conversations/{id}/members/{user_id}/alias", server.auth(server.setMemberAlias))
	mux.HandleFunc("POST /api/v1/conversations/{id}/members/{user_id}/mute", server.auth(server.setMemberMute))
	mux.HandleFunc("POST /api/v1/conversations/{id}/owner", server.auth(server.transferOwnership))
	mux.HandleFunc("POST /api/v1/conversations/{id}/roles", server.auth(server.setRole))
	mux.HandleFunc("PATCH /api/v1/conversations/{id}/settings", server.auth(server.settings))
	mux.HandleFunc("POST /api/v1/conversations/{id}/dismiss", server.auth(server.dismissGroup))
	mux.HandleFunc("POST /api/v1/conversations/{id}/poke", server.auth(server.poke))
	mux.HandleFunc("GET /api/v1/conversations/{id}/join-requests", server.auth(server.joinRequests))
	mux.HandleFunc("POST /api/v1/conversations/{id}/join-requests/{request_id}/{action}", server.auth(server.decideJoinRequest))
	mux.HandleFunc("GET /api/v1/conversations/{id}/announcements", server.auth(server.announcements))
	mux.HandleFunc("POST /api/v1/conversations/{id}/announcements", server.auth(server.createAnnouncement))
	mux.HandleFunc("PATCH /api/v1/conversations/{id}/announcements/{announcement_id}", server.auth(server.updateAnnouncement))
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/announcements/{announcement_id}", server.auth(server.deleteAnnouncement))
	mux.HandleFunc("PATCH /api/v1/conversations/{id}/preferences", server.auth(server.setPreferences))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages", server.auth(server.history))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages/search", server.auth(server.searchMessages))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages/{seq}/delivery", server.auth(server.deliveryStatus))
	mux.HandleFunc("GET /api/v1/conversations/{id}/sync", server.auth(server.sync))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages", server.auth(server.send))
	mux.HandleFunc("GET /api/v1/conversations/{id}/messages/{message_id}/thread", server.auth(server.thread))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages/{seq}/recall", server.auth(server.recall))
	mux.HandleFunc("POST /api/v1/conversations/{id}/messages/{message_id}/reactions", server.auth(server.react))
	mux.HandleFunc("DELETE /api/v1/conversations/{id}/messages/{message_id}/reactions", server.auth(server.unreact))
	mux.HandleFunc("POST /api/v1/conversations/{id}/media", server.auth(server.uploadMedia))
	mux.HandleFunc("GET /api/v1/media/{media_id}", server.auth(server.downloadMedia))
	mux.HandleFunc("POST /api/v1/conversations/{id}/read", server.auth(server.read))
	mux.HandleFunc("POST /api/v1/conversations/{id}/received", server.auth(server.received))
	mux.HandleFunc("GET /api/v1/devices/{device_id}/cursors", server.auth(server.cursors))
	mux.HandleFunc("POST /api/v1/presence", server.auth(server.presence))
	mux.HandleFunc("GET /ws", server.auth(server.websocket))
	return server.cors(server.csrf(mux))
}
