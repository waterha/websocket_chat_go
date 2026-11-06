package api

import (
	"errors"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

func (server *Server) createFavorite(w http.ResponseWriter, r *http.Request, c identity) {
	var in struct {
		ConversationID string `json:"conversation_id"`
		MessageID      string `json:"message_id"`
	}
	if !decode(w, r, &in) {
		return
	}
	item, err := server.Services.Favorites.Create(r.Context(), c.account.ID, in.ConversationID, in.MessageID)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 201, item)
}
func (server *Server) listFavorites(w http.ResponseWriter, r *http.Request, c identity) {
	limit := 30
	var err error
	if r.URL.Query().Has("limit") {
		limit, err = strconv.Atoi(r.URL.Query().Get("limit"))
		if err != nil {
			fail(w, model.Fail(400, "invalid_limit"))
			return
		}
	}
	page, err := server.Services.Favorites.List(r.Context(), c.account.ID, r.URL.Query().Get("before_id"), limit)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, page)
}
func (server *Server) deleteFavorite(w http.ResponseWriter, r *http.Request, c identity) {
	if err := server.Services.Favorites.Delete(r.Context(), c.account.ID, r.PathValue("id")); err != nil {
		fail(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
func (server *Server) downloadFavoriteMedia(w http.ResponseWriter, r *http.Request, c identity) {
	a, err := server.Services.Favorites.Media(r.Context(), c.account.ID, r.PathValue("id"), r.PathValue("media_id"))
	if err != nil {
		fail(w, err)
		return
	}
	f, err := server.Services.Favorites.Open(a)
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
	w.Header().Set("Cache-Control", "private, max-age=86400")
	inlineKind, allowed := logic.MediaKind(a.ContentType)
	if !allowed || inlineKind == "file" || inlineKind != a.MediaType {
		w.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(a.FileName))
	} else {
		w.Header().Set("Content-Disposition", "inline")
	}
	http.ServeContent(w, r, a.FileName, a.CreatedAt, f)
}
