package api

import (
	"errors"
	"net/http"
	"os"
	"strconv"
	"websocket_chat_go/backend/internal/model"
)

func (server *Server) createSticker(w http.ResponseWriter, r *http.Request, c identity) {
	var in struct {
		ConversationID string `json:"conversation_id"`
		MessageID      string `json:"message_id"`
		MediaID        string `json:"media_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	item, err := server.Services.Stickers.Create(r.Context(), c.account.ID, in.ConversationID, in.MessageID, in.MediaID)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 201, item)
}
func (server *Server) listStickers(w http.ResponseWriter, r *http.Request, c identity) {
	page, err := server.Services.Stickers.List(r.Context(), c.account.ID)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, page)
}
func (server *Server) deleteSticker(w http.ResponseWriter, r *http.Request, c identity) {
	if err := server.Services.Stickers.Delete(r.Context(), c.account.ID, r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (server *Server) prepareSticker(w http.ResponseWriter, r *http.Request, c identity) {
	var in struct {
		ConversationID string `json:"conversation_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	item, err := server.Services.Stickers.Prepare(r.Context(), c.account.ID, r.PathValue("id"), in.ConversationID)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 201, item)
}
func (server *Server) downloadStickerMedia(w http.ResponseWriter, r *http.Request, c identity) {
	a, err := server.Services.Stickers.Media(r.Context(), c.account.ID, r.PathValue("id"))
	if err != nil {
		fail(w, err)
		return
	}
	f, err := server.Services.Media.Files.Open(a.StorageKey)
	if errors.Is(err, os.ErrNotExist) {
		fail(w, model.Fail(404, "not_found"))
		return
	}
	if err != nil {
		fail(w, err)
		return
	}
	defer f.Close()
	w.Header().Set("Content-Type", a.ContentType)
	w.Header().Set("Content-Length", strconv.FormatInt(a.ByteSize, 10))
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Cache-Control", "private, no-store")
	w.Header().Set("Content-Disposition", "inline")
	http.ServeContent(w, r, a.FileName, a.CreatedAt, f)
}
