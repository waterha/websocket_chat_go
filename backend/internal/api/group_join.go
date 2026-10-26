package api

import (
	"net/http"
	"strings"

	"websocket_chat_go/backend/internal/model"
)

// discoverGroups returns active groups that are not already joined by caller.
func (server *Server) discoverGroups(writer http.ResponseWriter, request *http.Request, caller identity) {
	groups, err := server.Services.Chat.DiscoverGroups(request.Context(), caller.account.ID, request.URL.Query().Get("q"))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, groups)
}

// myJoinRequests exposes the caller's own application decision history only.
func (server *Server) myJoinRequests(writer http.ResponseWriter, request *http.Request, caller identity) {
	requests, err := server.Services.Chat.MyJoinRequests(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	items := make([]map[string]any, 0, len(requests))
	for _, joinRequest := range requests {
		items = append(items, map[string]any{
			"id": joinRequest.ID, "conversation_id": joinRequest.ConversationID,
			"group_name": joinRequest.GroupName, "message": joinRequest.Message,
			"status": joinRequest.Status,
		})
	}
	respond(writer, 200, items)
}

// requestToJoin either adds the caller immediately or creates an application.
func (server *Server) requestToJoin(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.JoinGroupInput
	if !decode(writer, request, &input) {
		return
	}
	joinRequest, err := server.Services.Chat.RequestToJoin(request.Context(), caller.account.ID, request.PathValue("id"), input.Message)
	if err != nil {
		fail(writer, err)
		return
	}
	if joinRequest.ID == "" {
		respond(writer, 200, map[string]any{"pending": false, "id": request.PathValue("id")})
		return
	}
	respond(writer, 200, map[string]any{"pending": true, "id": joinRequest.ID})
}

// cancelJoinRequest withdraws only this caller's pending application.
func (server *Server) cancelJoinRequest(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Chat.CancelJoinRequest(request.Context(), caller.account.ID, request.PathValue("id")); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": strings.TrimSpace(request.PathValue("id"))})
}
