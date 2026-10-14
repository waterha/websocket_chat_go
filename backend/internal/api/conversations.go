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
	respond(writer, 200, items)
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
