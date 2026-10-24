package api

import (
	"net/http"

	"websocket_chat_go/backend/internal/model"
)

func (server *Server) conversations(writer http.ResponseWriter, request *http.Request, caller identity) {
	items, err := server.Services.Chat.Conversations(request.Context(), caller.account.ID)
	if err != nil {
		fail(writer, err)
		return
	}
	// Mark online members so the client can show who is reachable now. The check
	// is best-effort: a Redis failure must not fail the whole list.
	userIDs := []string{}
	for _, item := range items {
		for _, member := range item.Members {
			userIDs = append(userIDs, member.ID)
		}
	}
	if online, err := server.Services.Presence.Online(request.Context(), userIDs); err == nil {
		for index := range items {
			for memberIndex := range items[index].Members {
				items[index].Members[memberIndex].Online = online[items[index].Members[memberIndex].ID]
			}
		}
	}
	respond(writer, 200, items)
}

// presence reports which of the given users currently hold a live connection.
func (server *Server) presence(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		UserIDs []string `json:"user_ids"`
	}
	if !decode(writer, request, &input) {
		return
	}
	if len(input.UserIDs) > 1000 {
		fail(writer, model.Fail(400, "too_many_users"))
		return
	}
	online, err := server.Services.Presence.Online(request.Context(), input.UserIDs)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]any{"online": online})
}

func (server *Server) create(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.CreateInput
	if !decode(writer, request, &input) {
		return
	}
	id, err := server.Services.Chat.Create(request.Context(), caller.account.ID, input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 201, map[string]string{"id": id})
}

func (server *Server) addMembers(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.AddMembersInput
	if !decode(writer, request, &input) {
		return
	}
	result, err := server.Services.Chat.AddMembers(request.Context(), caller.account.ID, request.PathValue("id"), input)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, result)
}

// deleteConversation removes one conversation from the caller's own sidebar.
// The other members keep theirs, and the next message restores it.
func (server *Server) deleteConversation(writer http.ResponseWriter, request *http.Request, caller identity) {
	if err := server.Services.Chat.DeleteConversation(request.Context(), caller.account.ID, request.PathValue("id")); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}

// poke sends a playful tap aimed at one member.
func (server *Server) poke(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input model.PokeInput
	if !decode(writer, request, &input) {
		return
	}
	if err := server.Services.Chat.Poke(request.Context(), caller.account.ID, request.PathValue("id"), input.UserID); err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, map[string]string{"id": request.PathValue("id")})
}
