package logic

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// Upload accepts the same declared image types as normal media uploads. The
// cleanup reservation is committed before taking any transaction connection.
func (s *StickerService) Upload(ctx context.Context, userID, fileName, contentType string, size int64, body io.Reader) (model.Sticker, error) {
	contentType = strings.ToLower(strings.TrimSpace(strings.Split(contentType, ";")[0]))
	if kind, ok := MediaKind(contentType); !ok || kind != "image" {
		return model.Sticker{}, model.Fail(415, "unsupported_media_type")
	}
	if size <= 0 {
		return model.Sticker{}, model.Fail(400, "invalid_upload")
	}
	if size > MaxImageBytes {
		return model.Sticker{}, model.Fail(413, "media_too_large")
	}
	// Validate the signature before registering or writing any asset. The
	// bounded prefix rejects HTML/SVG disguised as an allowed image type.
	prefix := make([]byte, min(size, 512))
	if _, err := io.ReadFull(body, prefix); err != nil {
		return model.Sticker{}, model.Fail(400, "invalid_upload")
	}
	if http.DetectContentType(prefix) != contentType {
		return model.Sticker{}, model.Fail(415, "unsupported_media_type")
	}
	body = io.MultiReader(bytes.NewReader(prefix), body)
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	key, err := s.reserveCopy(ctx)
	if err != nil {
		return model.Sticker{}, err
	}
	out, err := s.Files.Create(key)
	if err != nil {
		return model.Sticker{}, err
	}
	hash := sha256.New()
	n, err := io.Copy(io.MultiWriter(out, hash), io.LimitReader(body, size+1))
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err != nil {
		_ = s.Files.Remove(key)
		return model.Sticker{}, err
	}
	if n != size {
		_ = s.Files.Remove(key)
		return model.Sticker{}, model.Fail(400, "invalid_upload")
	}
	source := "sha256:" + hex.EncodeToString(hash.Sum(nil))
	tx, err := s.Store.DB.Begin(ctx)
	if err != nil {
		return model.Sticker{}, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "stickers:"+userID); err != nil {
		return model.Sticker{}, err
	}
	existing, err := scanSticker(tx.QueryRow(ctx, "SELECT "+stickerColumns+" FROM stickers WHERE user_id=$1 AND kind='image' AND source_key=$2", userID, source))
	if err == nil {
		_ = s.Files.Remove(key)
		return existing, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Sticker{}, err
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM stickers WHERE user_id=$1", userID).Scan(&count); err != nil {
		return model.Sticker{}, err
	}
	if count >= 200 {
		_ = s.Files.Remove(key)
		return model.Sticker{}, model.Fail(409, "sticker_limit")
	}
	item, err := scanSticker(tx.QueryRow(ctx, "INSERT INTO stickers(id,user_id,kind,source_key,file_name,content_type,byte_size,storage_key) VALUES($1,$2,'image',$3,$4,$5,$6,$7) RETURNING "+stickerColumns, model.NewID(), userID, source, cleanFileName(fileName), contentType, size, key))
	if err != nil {
		return model.Sticker{}, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM favorite_asset_cleanup WHERE storage_key=$1", key); err != nil {
		return model.Sticker{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Sticker{}, err
	}
	return item, nil
}
