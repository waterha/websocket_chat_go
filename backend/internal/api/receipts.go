package api

import (
	"net/http"
	"strconv"
	"websocket_chat_go/backend/internal/model"
)

func (server *Server) read(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		Seq int64 `json:"seq"`
	}
	if !decode(writer, request, &input) {
		return
	}
	receipt, err := server.Services.Chat.MarkRead(request.Context(), caller.account.ID, request.PathValue("id"), input.Seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, receipt)
}

func (server *Server) received(writer http.ResponseWriter, request *http.Request, caller identity) {
	var input struct {
		Seq      int64  `json:"seq"`
		DeviceID string `json:"device_id"`
	}
	if !decode(writer, request, &input) {
		return
	}
	cursor, err := server.Services.Chat.MarkReceived(request.Context(), caller.account.ID, request.PathValue("id"), input.DeviceID, input.Seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, cursor)
}

func (server *Server) deliveryStatus(writer http.ResponseWriter, request *http.Request, caller identity) {
	seq, err := strconv.ParseInt(request.PathValue("seq"), 10, 64)
	if err != nil {
		fail(writer, model.Fail(404, "not_found"))
		return
	}
	status, err := server.Services.Chat.DeliveryStatus(request.Context(), caller.account.ID, request.PathValue("id"), seq)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, status)
}

func (server *Server) cursors(writer http.ResponseWriter, request *http.Request, caller identity) {
	items, err := server.Services.Chat.Cursors(request.Context(), caller.account.ID, request.PathValue("device_id"))
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 200, items)
}
