package api

import (
	"math"
	"net/http"
	"strconv"
	"websocket_chat_go/backend/internal/model"
)

func integer(request *http.Request, key string, fallback int64) (int64, error) {
	value := request.URL.Query().Get(key)
	if value == "" {
		return fallback, nil
	}
	number, err := strconv.ParseInt(value, 10, 64)
	if err != nil {
		return 0, model.Fail(400, "invalid_cursor")
	}
	return number, nil
}

func (server *Server) history(writer http.ResponseWriter, request *http.Request, caller identity) {
	before, err := integer(request, "before_seq", math.MaxInt64)
	if err != nil {
		fail(writer, err)
		return
	}
	limit, err := integer(request, "limit", 30)
	if err != nil {
		fail(writer, err)
		return
	}
	if limit < 1 || limit > 100 {
		fail(writer, model.Fail(400, "invalid_limit"))
		return
	}
	items, err := server.Services.Chat.History(request.Context(), caller.account.ID, request.PathValue("id"), before, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, items)
}

func (server *Server) sync(writer http.ResponseWriter, request *http.Request, caller identity) {
	after, err := integer(request, "after_seq", 0)
	if err != nil {
		fail(writer, err)
		return
	}
	limit, err := integer(request, "limit", 100)
	if err != nil {
		fail(writer, err)
		return
	}
	if limit < 1 || limit > 100 {
		fail(writer, model.Fail(400, "invalid_limit"))
		return
	}
	page, err := server.Services.Chat.Sync(request.Context(), caller.account.ID, request.PathValue("id"), after, int(limit))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, page)
}

func (server *Server) send(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		ClientMsgID string `json:"client_msg_id"`
		Content     string `json:"content"`
	}
	if !decode(writer, request, &input) {
		return
	}
	item, duplicate, err := server.Services.Chat.Send(request.Context(), caller.account.ID, model.SendInput{ConversationID: request.PathValue("id"), ClientMsgID: input.ClientMsgID, Content: input.Content})
	if err != nil {
		fail(writer, err)
		return
	}
	status := 201
	if duplicate {
		status = 200
	}
	respond(writer, status, item)
}
