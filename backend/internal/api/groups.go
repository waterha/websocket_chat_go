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

// announcements lists every notice of a group. Any member may read them.
func (server *Server) announcements(writer http.ResponseWriter, request *http.Request, caller identity) {
	items, err := server.Services.Chat.Announcements(request.Context(), caller.account.ID, request.PathValue("id"))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, items)
}

func (server *Server) createAnnouncement(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.AnnouncementInput
	if !decode(writer, request, &input) {
		return
	}
	item, err := server.Services.Chat.CreateAnnouncement(request.Context(), caller.account.ID, request.PathValue("id"), input.Content)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 201, item)
}

func (server *Server) updateAnnouncement(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.AnnouncementInput
	if !decode(writer, request, &input) {
		return
	}
	item, err := server.Services.Chat.UpdateAnnouncement(request.Context(), caller.account.ID, request.PathValue("id"), request.PathValue("announcement_id"), input.Content)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, item)
}

func (server *Server) deleteAnnouncement(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Chat.DeleteAnnouncement(request.Context(), caller.account.ID, request.PathValue("id"), request.PathValue("announcement_id")); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("announcement_id")})
}

// setMemberAlias sets the group card of one member.
func (server *Server) setMemberAlias(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.MemberAliasInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Chat.SetMemberAlias(request.Context(), caller.account.ID, request.PathValue("id"), request.PathValue("user_id"), input.Alias); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("user_id")})
}

// setMemberMute silences one member for the given minutes; zero lifts it.
func (server *Server) setMemberMute(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.MemberMuteInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Chat.SetMemberMute(request.Context(), caller.account.ID, request.PathValue("id"), request.PathValue("user_id"), input.Minutes); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("user_id")})
}

// settings flips the group-wide toggles and reports the values in force.
func (server *Server) settings(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.ConversationSettingsInput
	if !decode(writer, request, &input) {
		return
	}
	settings, err := server.Services.Chat.Settings(request.Context(), caller.account.ID, request.PathValue("id"), input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, settings)
}

// dismissGroup closes the group for good. Only the owner may call it.
func (server *Server) dismissGroup(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Chat.DismissGroup(request.Context(), caller.account.ID, request.PathValue("id")); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}

// joinRequests lists the invites awaiting an admin decision.
func (server *Server) joinRequests(writer http.ResponseWriter, request *http.Request, caller identity) {
	requests, err := server.Services.Chat.JoinRequests(request.Context(), caller.account.ID, request.PathValue("id"))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, requests)
}

// decideJoinRequest accepts or rejects one pending invite.
func (server *Server) decideJoinRequest(writer http.ResponseWriter, request *http.Request, caller identity) {
	joinRequest, accepted, err := server.Services.Chat.DecideJoinRequest(request.Context(), caller.account.ID, request.PathValue("id"), request.PathValue("request_id"), request.PathValue("action"))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]any{"join_request": joinRequest, "accepted": accepted})
}
