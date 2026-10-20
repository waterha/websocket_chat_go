package api

import (
	"net/http"

	"websocket_chat_go/backend/internal/model"
)

// removeMember handles both "remove someone" and "leave": the target is taken
// from the body for removals and from the caller for leaving, so a member cannot
// remove another user by mistake.
func (server *Server) removeMember(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.RemoveMemberInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Chat.RemoveMember(request.Context(), caller.account.ID, request.PathValue("id"), input.UserID); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}

// leaveGroup removes the caller from a group without naming a target.
func (server *Server) leaveGroup(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Chat.RemoveMember(request.Context(), caller.account.ID, request.PathValue("id"), caller.account.ID); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}

func (server *Server) setRole(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.SetRoleInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Chat.SetRole(request.Context(), caller.account.ID, request.PathValue("id"), input.UserID, input.Role); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}

func (server *Server) transferOwnership(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.TransferInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Chat.TransferOwnership(request.Context(), caller.account.ID, request.PathValue("id"), input.UserID); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}

func (server *Server) setPreferences(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.PreferenceInput
	if !decode(writer, request, &input) {
		return
	}
	preferences, err := server.Services.Chat.Preferences(request.Context(), caller.account.ID, request.PathValue("id"), input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, preferences)
}

func (server *Server) setAnnouncement(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.AnnouncementInput
	if !decode(writer, request, &input) {
		return
	}
	announcement, err := server.Services.Chat.SetAnnouncement(request.Context(), caller.account.ID, request.PathValue("id"), input.Announcement)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"announcement": announcement})
}
