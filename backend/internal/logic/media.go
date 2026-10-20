package logic

import (
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
)

const (
	MaxImageBytes = 10 << 20
	MaxVideoBytes = 100 << 20
	MaxFileBytes  = 50 << 20
)

// allowedMedia maps a declared content type to the message kind it produces.
// Images and videos are inline kinds; anything else becomes a downloadable
// file, which is why the set is not a strict allow-list of media types.
var allowedMedia = map[string]string{
	"image/jpeg": "image",
	"image/png":  "image",
	"image/gif":  "image",
	"image/webp": "image",
	"video/mp4":  "video",
	"video/webm": "video",
	"video/ogg":  "video",
}

type MediaService struct {
	Store *model.Store
	Files *media.Store
}

// MediaKind resolves the message kind for a declared content type. Known image
// and video types keep their inline kind; a syntactically valid other type is
// stored as a generic file, so documents and archives can be shared. A malformed
// type (empty, too long, control characters, or not `type/subtype`) is refused.
func MediaKind(contentType string) (string, bool) {
	cleaned := strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if !validContentType(cleaned) {
		return "", false
	}
	if kind, known := allowedMedia[cleaned]; known {
		return kind, true
	}
	return "file", true
}

// validContentType accepts only the `type/subtype` shape with the restricted
// token alphabet from RFC 9110, which keeps stored values free of whitespace,
// parameters and control characters.
func validContentType(value string) bool {
	if value == "" || len(value) > 128 || strings.Count(value, "/") != 1 {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" {
			return false
		}
		for _, digit := range part {
			switch {
			case digit >= 'a' && digit <= 'z':
			case digit >= '0' && digit <= '9':
			case digit == '!' || digit == '#' || digit == '$' || digit == '&' || digit == '\'' || digit == '*' || digit == '+' || digit == '-' || digit == '.' || digit == '^' || digit == '_' || digit == '`' || digit == '|' || digit == '~':
			default:
				return false
			}
		}
	}
	return true
}

func MediaLimit(kind string) int64 {
	switch kind {
	case "video":
		return MaxVideoBytes
	case "file":
		return MaxFileBytes
	default:
		return MaxImageBytes
	}
}

func cleanFileName(name string) string {
	name = strings.TrimSpace(filepath.Base(strings.ReplaceAll(name, "\\", "/")))
	if name == "." || name == "/" || name == "" {
		name = "upload"
	}
	if utf8.RuneCountInString(name) > 255 {
		runes := []rune(name)
		name = string(runes[:255])
	}
	return name
}

// Upload stores the bytes first and records ownership afterwards, so a failed
// database write cannot leave a row pointing at a missing file.
func (service *MediaService) Upload(ctx context.Context, userID, conversationID, fileName, contentType, kind string, size int64, body io.Reader) (model.MediaUpload, error) {
	upload := model.MediaUpload{
		ID:             model.NewID(),
		OwnerID:        userID,
		ConversationID: conversationID,
		FileName:       cleanFileName(fileName),
		MediaType:      kind,
		ContentType:    contentType,
		ByteSize:       size,
		StorageKey:     model.NewID(),
	}
	var member bool
	if err := service.Store.DB.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", conversationID, userID).Scan(&member); err != nil {
		return model.MediaUpload{}, err
	}
	if !member {
		return model.MediaUpload{}, model.Fail(404, "not_found")
	}
	file, err := service.Files.Create(upload.StorageKey)
	if err != nil {
		return model.MediaUpload{}, err
	}
	written, copyErr := io.Copy(file, body)
	closeErr := file.Close()
	if copyErr != nil || closeErr != nil {
		_ = service.Files.Remove(upload.StorageKey)
		if copyErr != nil {
			return model.MediaUpload{}, copyErr
		}
		return model.MediaUpload{}, closeErr
	}
	if written != size {
		_ = service.Files.Remove(upload.StorageKey)
		return model.MediaUpload{}, model.Fail(400, "invalid_upload")
	}
	if _, err = service.Store.DB.Exec(ctx, "INSERT INTO message_media(id,owner_id,conversation_id,file_name,media_type,content_type,byte_size,storage_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8)", upload.ID, upload.OwnerID, upload.ConversationID, upload.FileName, upload.MediaType, upload.ContentType, upload.ByteSize, upload.StorageKey); err != nil {
		_ = service.Files.Remove(upload.StorageKey)
		return model.MediaUpload{}, err
	}
	return upload, nil
}

// Retrieve returns the upload only when the caller is a member of the owning
// conversation, so media URLs cannot be shared outside their conversation.
func (service *MediaService) Retrieve(ctx context.Context, userID, mediaID string) (model.MediaUpload, error) {
	upload := model.MediaUpload{}
	err := service.Store.DB.QueryRow(ctx, `SELECT m.id,m.owner_id,m.conversation_id,m.file_name,m.media_type,m.content_type,m.byte_size,m.storage_key,m.created_at
		FROM message_media m JOIN conversation_members cm ON cm.conversation_id=m.conversation_id
		WHERE m.id=$1 AND cm.user_id=$2`, mediaID, userID).Scan(&upload.ID, &upload.OwnerID, &upload.ConversationID, &upload.FileName, &upload.MediaType, &upload.ContentType, &upload.ByteSize, &upload.StorageKey, &upload.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return model.MediaUpload{}, model.Fail(404, "not_found")
	}
	return upload, err
}

// Open returns the stored bytes for an already authorised upload.
func (service *MediaService) Open(upload model.MediaUpload) (*os.File, error) {
	return service.Files.Open(upload.StorageKey)
}
