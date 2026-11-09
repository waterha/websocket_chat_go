package logic

import (
	"context"
	"errors"
	"io"
	"time"
	"unicode"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
)

type StickerService struct {
	Store *model.Store
	Files *media.Store
}

const stickerColumns = "id,kind,text,file_name,byte_size,created_at"

func scanSticker(row pgx.Row) (s model.Sticker, err error) {
	err = row.Scan(&s.ID, &s.Kind, &s.Text, &s.FileName, &s.ByteSize, &s.CreatedAt)
	return
}

// EmojiOnly intentionally accepts a conservative Unicode subset, never prose,
// markup or URLs. Joiners/modifiers must follow a base; dangling ZWJ is rejected.
func EmojiOnly(text string) bool {
	if !utf8.ValidString(text) || len(text) == 0 || utf8.RuneCountInString(text) > 64 {
		return false
	}
	base, join := false, false
	runes := []rune(text)
	for i := 0; i < len(runes); i++ {
		r := runes[i]
		switch {
		case unicode.IsSpace(r):
			if join {
				return false
			}
		case r == '#' || r == '*' || (r >= '0' && r <= '9'):
			if i+1 < len(runes) && runes[i+1] == 0xFE0F {
				i++
			}
			if i+1 >= len(runes) || runes[i+1] != 0x20E3 {
				return false
			}
			i++
			base = true
			join = false
		case r == 0x200D:
			if !base || join {
				return false
			}
			join = true
		case r == 0xFE0F || r == 0xFE0E || (r >= 0x1F3FB && r <= 0x1F3FF):
			if !base || join {
				return false
			}
		case (r >= 0x1F300 && r <= 0x1FAFF) || (r >= 0x1F1E6 && r <= 0x1F1FF) || (r >= 0x2600 && r <= 0x27BF) || r == 0x231A || r == 0x231B || r == 0x23F0 || r == 0x23F3 || r == 0x2B50 || r == 0x2B55 || r == 0x00A9 || r == 0x00AE || r == 0x203C || r == 0x2049 || r == 0x2122 || r == 0x2139 || (r >= 0x2194 && r <= 0x2199) || r == 0x21A9 || r == 0x21AA || (r >= 0x23E9 && r <= 0x23F3) || (r >= 0x23F8 && r <= 0x23FA) || r == 0x24C2 || r == 0x25AA || r == 0x25AB || r == 0x25B6 || r == 0x25C0 || (r >= 0x25FB && r <= 0x25FE) || r == 0x2934 || r == 0x2935 || (r >= 0x2B05 && r <= 0x2B07) || r == 0x2B1B || r == 0x2B1C || r == 0x3030 || r == 0x303D || r == 0x3297 || r == 0x3299:
			base = true
			join = false
		default:
			return false
		}
	}
	return base && !join
}

func (s *StickerService) Create(ctx context.Context, userID, cid, mid, mediaID string) (model.Sticker, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	copyKey, err := s.reserveCopy(ctx)
	if err != nil {
		return model.Sticker{}, err
	}
	tx, err := s.Store.DB.Begin(ctx)
	if err != nil {
		return model.Sticker{}, err
	}
	defer tx.Rollback(ctx)
	var locked string
	if err = tx.QueryRow(ctx, "SELECT id FROM conversations WHERE id=$1 FOR UPDATE", cid).Scan(&locked); err != nil {
		return model.Sticker{}, stickerError(err)
	}
	if err = tx.QueryRow(ctx, "SELECT user_id FROM conversation_members WHERE conversation_id=$1 AND user_id=$2 AND deleted_at IS NULL FOR KEY SHARE", cid, userID).Scan(&locked); err != nil {
		return model.Sticker{}, stickerError(err)
	}
	m, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE id=$1 AND conversation_id=$2 FOR UPDATE", mid, cid))
	if err != nil {
		return model.Sticker{}, stickerError(err)
	}
	if m.RecalledAt != nil || m.Kind == "system" {
		return model.Sticker{}, model.Fail(400, "cannot_sticker")
	}
	images := map[string]bool{}
	if m.Kind == "image" && m.MediaID != "" {
		images[m.MediaID] = true
	}
	for _, b := range m.Blocks {
		if b.Type == "image" && b.MediaID != "" {
			images[b.MediaID] = true
		}
	}
	item := model.Sticker{ID: model.NewID(), Kind: "emoji", Text: m.Content}
	source := m.Content
	if mediaID == "" && len(images) == 1 {
		for id := range images {
			mediaID = id
		}
	}
	var a model.MediaUpload
	if mediaID != "" {
		if !images[mediaID] {
			return model.Sticker{}, model.Fail(400, "invalid_sticker_media")
		}
		err = tx.QueryRow(ctx, "SELECT file_name,media_type,content_type,byte_size,storage_key FROM message_media WHERE id=$1 AND conversation_id=$2", mediaID, cid).Scan(&a.FileName, &a.MediaType, &a.ContentType, &a.ByteSize, &a.StorageKey)
		if err != nil {
			return model.Sticker{}, stickerError(err)
		}
		kind, ok := MediaKind(a.ContentType)
		if !ok || kind != "image" || a.MediaType != "image" || a.ByteSize <= 0 || a.ByteSize > MaxImageBytes {
			return model.Sticker{}, model.Fail(400, "invalid_sticker_media")
		}
		item.Kind = "image"
		item.Text = ""
		item.FileName = a.FileName
		item.ByteSize = a.ByteSize
		source = mediaID
	} else if len(images) > 0 || m.MediaID != "" || !EmojiOnly(m.Content) {
		return model.Sticker{}, model.Fail(400, "invalid_sticker_emoji")
	}
	// Serialize this user's dedup/count across all source conversations.
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1,0))", "stickers:"+userID); err != nil {
		return model.Sticker{}, err
	}
	existing, err := scanSticker(tx.QueryRow(ctx, "SELECT "+stickerColumns+" FROM stickers WHERE user_id=$1 AND kind=$2 AND source_key=$3", userID, item.Kind, source))
	if err == nil {
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
		return model.Sticker{}, model.Fail(409, "sticker_limit")
	}
	var key any
	if item.Kind == "image" {
		k, e := s.copyAsset(ctx, a.StorageKey, a.ByteSize, copyKey)
		if e != nil {
			return model.Sticker{}, e
		}
		key = k
	}
	item, err = scanSticker(tx.QueryRow(ctx, "INSERT INTO stickers(id,user_id,kind,source_key,text,file_name,content_type,byte_size,storage_key) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9) RETURNING "+stickerColumns, item.ID, userID, item.Kind, source, item.Text, item.FileName, a.ContentType, item.ByteSize, key))
	if err != nil {
		return model.Sticker{}, err
	}
	if key != nil {
		if _, err = tx.Exec(ctx, "DELETE FROM favorite_asset_cleanup WHERE storage_key=$1", key); err != nil {
			return model.Sticker{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Sticker{}, err
	}
	return item, nil
}
func stickerError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Fail(404, "not_found")
	}
	return err
}

// reserveCopy must run before Begin: borrowing another pool connection while
// holding a conversation lock can deadlock when waiters exhaust the pool.
func (s *StickerService) reserveCopy(ctx context.Context) (string, error) {
	key := model.NewID()
	_, err := s.Store.DB.Exec(ctx, "INSERT INTO favorite_asset_cleanup(storage_key,ready_at) VALUES($1,now()+interval '1 hour')", key)
	return key, err
}

// The pre-registered orphan survives rollback and unknown commit outcomes.
func (s *StickerService) copyAsset(ctx context.Context, old string, size int64, key string) (string, error) {
	in, err := s.Files.Open(old)
	if err != nil {
		return "", err
	}
	defer in.Close()
	out, err := s.Files.Create(key)
	if err != nil {
		return "", err
	}
	n, err := io.Copy(out, io.LimitReader(in, size+1))
	closeErr := out.Close()
	if err == nil {
		err = closeErr
	}
	if err == nil && n != size {
		err = io.ErrUnexpectedEOF
	}
	if err != nil {
		_ = s.Files.Remove(key)
		return "", err
	}
	return key, nil
}
func (s *StickerService) List(ctx context.Context, userID string) (model.StickerPage, error) {
	page := model.StickerPage{Stickers: []model.Sticker{}}
	rows, err := s.Store.DB.Query(ctx, "SELECT "+stickerColumns+" FROM stickers WHERE user_id=$1 ORDER BY created_at DESC,id DESC", userID)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		item, e := scanSticker(rows)
		if e != nil {
			return page, e
		}
		page.Stickers = append(page.Stickers, item)
	}
	return page, rows.Err()
}
func (s *StickerService) Delete(ctx context.Context, userID, id string) error {
	tag, err := s.Store.DB.Exec(ctx, "DELETE FROM stickers WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	return (&FavoriteService{Store: s.Store, Files: s.Files}).Clean(ctx)
}
func (s *StickerService) Media(ctx context.Context, userID, id string) (a model.MediaUpload, err error) {
	err = s.Store.DB.QueryRow(ctx, "SELECT id,file_name,'image',content_type,byte_size,storage_key,created_at FROM stickers WHERE id=$1 AND user_id=$2 AND kind='image'", id, userID).Scan(&a.ID, &a.FileName, &a.MediaType, &a.ContentType, &a.ByteSize, &a.StorageKey, &a.CreatedAt)
	return a, stickerError(err)
}

func (s *StickerService) Prepare(ctx context.Context, userID, id, cid string) (model.Media, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	copyKey, err := s.reserveCopy(ctx)
	if err != nil {
		return model.Media{}, err
	}
	tx, err := s.Store.DB.Begin(ctx)
	if err != nil {
		return model.Media{}, err
	}
	defer tx.Rollback(ctx)
	var kind string
	var dismissed, allMuted bool
	if err = tx.QueryRow(ctx, "SELECT kind,dismissed_at IS NOT NULL,all_muted FROM conversations WHERE id=$1 FOR UPDATE", cid).Scan(&kind, &dismissed, &allMuted); err != nil {
		return model.Media{}, stickerError(err)
	}
	var role string
	var muted *time.Time
	if err = tx.QueryRow(ctx, "SELECT role,muted_until FROM conversation_members WHERE conversation_id=$1 AND user_id=$2 FOR KEY SHARE", cid, userID).Scan(&role, &muted); err != nil {
		return model.Media{}, stickerError(err)
	}
	if dismissed {
		return model.Media{}, model.Fail(403, "conversation_dismissed")
	}
	if muted != nil && muted.After(time.Now()) {
		return model.Media{}, model.Fail(403, "member_muted")
	}
	if allMuted && role == model.RoleMember {
		return model.Media{}, model.Fail(403, "conversation_muted")
	}
	if kind == "direct" {
		var other string
		if err = tx.QueryRow(ctx, "SELECT user_id FROM conversation_members WHERE conversation_id=$1 AND user_id<>$2", cid, userID).Scan(&other); err != nil {
			return model.Media{}, err
		}
		blocked, e := blockedBetween(ctx, tx, other, userID)
		if e != nil {
			return model.Media{}, e
		}
		if blocked {
			return model.Media{}, model.Fail(403, "blocked_by_user")
		}
	}
	var a model.MediaUpload
	if err = tx.QueryRow(ctx, "SELECT file_name,content_type,byte_size,storage_key FROM stickers WHERE id=$1 AND user_id=$2 AND kind='image' FOR SHARE", id, userID).Scan(&a.FileName, &a.ContentType, &a.ByteSize, &a.StorageKey); err != nil {
		return model.Media{}, stickerError(err)
	}
	key, err := s.copyAsset(ctx, a.StorageKey, a.ByteSize, copyKey)
	if err != nil {
		return model.Media{}, err
	}
	out := model.Media{ID: model.NewID(), FileName: a.FileName, MediaType: "image", ContentType: a.ContentType, ByteSize: a.ByteSize}
	if _, err = tx.Exec(ctx, "INSERT INTO message_media(id,owner_id,conversation_id,file_name,media_type,content_type,byte_size,storage_key) VALUES($1,$2,$3,$4,'image',$5,$6,$7)", out.ID, userID, cid, out.FileName, out.ContentType, out.ByteSize, key); err != nil {
		return model.Media{}, err
	}
	if _, err = tx.Exec(ctx, "DELETE FROM favorite_asset_cleanup WHERE storage_key=$1", key); err != nil {
		return model.Media{}, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Media{}, err
	}
	return out, nil
}
