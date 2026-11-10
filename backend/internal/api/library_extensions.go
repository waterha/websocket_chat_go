package api

import (
	"encoding/json"
	"net/http"
	"strings"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

func (server *Server) uploadSticker(w http.ResponseWriter, r *http.Request, c identity) {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(r.Header.Get("Content-Type"), ";")[0]))
	kind, ok := logic.MediaKind(contentType)
	if !ok || kind != "image" {
		fail(w, model.Fail(415, "unsupported_media_type"))
		return
	}
	if r.ContentLength <= 0 {
		fail(w, model.Fail(400, "invalid_upload"))
		return
	}
	if r.ContentLength > logic.MaxImageBytes {
		fail(w, model.Fail(413, "media_too_large"))
		return
	}
	name := r.URL.Query().Get("file_name")
	if name == "" {
		name = "upload" + extensionFor(contentType)
	}
	r.Body = http.MaxBytesReader(w, r.Body, logic.MaxImageBytes+1)
	item, err := server.Services.Stickers.Upload(r.Context(), c.account.ID, name, contentType, r.ContentLength, r.Body)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 201, item)
}

func (server *Server) patchFavorite(w http.ResponseWriter, r *http.Request, c identity) {
	// RawMessage distinguishes omitted fields from null: null is not an update.
	var in struct {
		Tags json.RawMessage `json:"tags"`
		Note json.RawMessage `json:"note"`
	}
	if !decode(w, r, &in) {
		return
	}
	var tags *[]string
	var note *string
	if in.Tags != nil {
		var v []string
		if string(in.Tags) == "null" || json.Unmarshal(in.Tags, &v) != nil {
			fail(w, model.Fail(400, "invalid_tags"))
			return
		}
		// Reject null array members, which encoding/json otherwise turns into "".
		var values []json.RawMessage
		_ = json.Unmarshal(in.Tags, &values)
		for _, value := range values {
			if string(value) == "null" {
				fail(w, model.Fail(400, "invalid_tags"))
				return
			}
		}
		tags = &v
	}
	if in.Note != nil {
		var v string
		if string(in.Note) == "null" || json.Unmarshal(in.Note, &v) != nil {
			fail(w, model.Fail(400, "invalid_note"))
			return
		}
		note = &v
	}
	item, err := server.Services.Favorites.Patch(r.Context(), c.account.ID, r.PathValue("id"), tags, note)
	if err != nil {
		fail(w, err)
		return
	}
	respond(w, 200, item)
}
