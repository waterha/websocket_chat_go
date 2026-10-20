package api

import (
	"errors"
	"mime"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

// maxUploadBody bounds the request reader before any media type is known. The
// per-type limits enforced afterwards are always smaller or equal.
const maxUploadBody = logic.MaxVideoBytes + (1 << 20)
// uploadMedia accepts a raw image, video or document body for one conversation.
// The media type is taken from the declared Content-Type, which is the same value
// later returned on download and stored verbatim. It is not content sniffed, so
// a client could label arbitrary bytes as an image; the endpoint therefore only
// guards size, type shape, conversation membership and sender ownership, and
// browsers must treat downloaded bytes as untrusted. Unknown types are stored as
// generic files rather than rejected.
func (server *Server) uploadMedia(writer http.ResponseWriter, request *http.Request, caller identity) {
	contentType := strings.ToLower(strings.TrimSpace(strings.Split(request.Header.Get("Content-Type"), ";")[0]))
	kind, ok := logic.MediaKind(contentType)
	if !ok {
		fail(writer, model.Fail(415, "unsupported_media_type"))
		return
	}
	size := request.ContentLength
	if size <= 0 {
		fail(writer, model.Fail(400, "invalid_upload"))
		return
	}
	if size > logic.MediaLimit(kind) {
		fail(writer, model.Fail(413, "media_too_large"))
		return
	}
	fileName := request.URL.Query().Get("file_name")
	if fileName == "" {
		fileName = "upload" + extensionFor(contentType)
	}
	request.Body = http.MaxBytesReader(writer, request.Body, maxUploadBody)
	upload, err := server.Services.Media.Upload(request.Context(), caller.account.ID, request.PathValue("id"), fileName, contentType, kind, size, request.Body)
	if err != nil {
		fail(writer, err)
		return
	}
	respond(writer, 201, upload)
}

// downloadMedia streams stored bytes only to a member of the owning
// conversation. The storage key never appears in any JSON response, so this
// endpoint is the single authorised path to the content.
func (server *Server) downloadMedia(writer http.ResponseWriter, request *http.Request, caller identity) {
	upload, err := server.Services.Media.Retrieve(request.Context(), caller.account.ID, request.PathValue("media_id"))
	if err != nil {
		fail(writer, err)
		return
	}
	file, err := server.Services.Media.Open(upload)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fail(writer, model.Fail(404, "not_found"))
			return
		}
		fail(writer, err)
		return
	}
	defer file.Close()
	writer.Header().Set("Content-Type", upload.ContentType)
	writer.Header().Set("Content-Length", strconv.FormatInt(upload.ByteSize, 10))
	// Inline only for real media; anything else is offered as a download so the
	// browser never renders an untrusted document in the app's origin.
	if upload.MediaType == "file" {
		writer.Header().Set("Content-Disposition", "attachment; filename*=UTF-8''"+url.PathEscape(upload.FileName))
	} else {
		writer.Header().Set("Content-Disposition", "inline")
	}
	writer.Header().Set("Cache-Control", "private, max-age=86400")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(writer, request, upload.FileName, upload.CreatedAt, file)
}

func extensionFor(contentType string) string {
	if extension, ok := map[string]string{
		"image/jpeg": ".jpg",
		"image/png":  ".png",
		"image/gif":  ".gif",
		"image/webp": ".webp",
		"video/mp4":  ".mp4",
		"video/webm": ".webm",
		"video/ogg":  ".ogv",
	}[contentType]; ok {
		return extension
	}
	extensions, err := mime.ExtensionsByType(contentType)
	if err == nil && len(extensions) > 0 {
		return extensions[0]
	}
	return filepath.Ext(contentType)
}
