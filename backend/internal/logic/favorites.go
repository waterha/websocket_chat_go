package logic

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
)

type FavoriteService struct {
	Store *model.Store
	Files *media.Store
}

type favoriteAsset struct {
	mediaID, fileName, mediaType, contentType, key string
	size                                           int64
}

func (service *FavoriteService) Create(ctx context.Context, userID, conversationID, messageID string) (model.Favorite, error) {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Minute)
	defer cancel()
	tx, err := service.Store.DB.Begin(ctx)
	if err != nil {
		return model.Favorite{}, err
	}
	defer tx.Rollback(ctx)
	var kind, name string
	// Match Recall and membership changes: lock conversation first, then read
	// membership in a fresh statement after a possible lock wait.
	if err = tx.QueryRow(ctx, `SELECT kind,name FROM conversations WHERE id=$1 FOR UPDATE`, conversationID).Scan(&kind, &name); errors.Is(err, pgx.ErrNoRows) {
		return model.Favorite{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Favorite{}, err
	}
	var member string
	if err = tx.QueryRow(ctx, `SELECT user_id FROM conversation_members WHERE conversation_id=$1 AND user_id=$2 AND deleted_at IS NULL FOR KEY SHARE`, conversationID, userID).Scan(&member); errors.Is(err, pgx.ErrNoRows) {
		return model.Favorite{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Favorite{}, err
	}
	var existing model.Favorite
	var raw []byte
	err = tx.QueryRow(ctx, `SELECT id,message,sender_name,sender_avatar,conversation_name,conversation_kind,saved_at,is_own FROM favorites WHERE user_id=$1 AND message_id=$2 AND conversation_id=$3`, userID, messageID, conversationID).Scan(&existing.ID, &raw, &existing.SenderName, &existing.SenderAvatar, &existing.ConversationName, &existing.ConversationKind, &existing.SavedAt, &existing.IsOwn)
	if err == nil {
		err = json.Unmarshal(raw, &existing.Message)
		return existing, err
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Favorite{}, err
	}
	var message model.Message
	message, err = scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE id=$1 AND conversation_id=$2 FOR UPDATE", messageID, conversationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Favorite{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Favorite{}, err
	}
	if message.RecalledAt != nil || message.Kind == "system" {
		return model.Favorite{}, model.Fail(400, "cannot_favorite")
	}
	if kind == "direct" && name == "" {
		if err = tx.QueryRow(ctx, `SELECT COALESCE((SELECT u.name FROM conversation_members cm JOIN users u ON u.id=cm.user_id WHERE cm.conversation_id=$1 AND cm.user_id<>$2 ORDER BY cm.user_id LIMIT 1),(SELECT name FROM users WHERE id=$2))`, conversationID, userID).Scan(&name); err != nil {
			return model.Favorite{}, err
		}
	}
	var senderName, senderAvatar string
	if err = tx.QueryRow(ctx, "SELECT name,COALESCE(avatar_media_id,'') FROM users WHERE id=$1", message.SenderID).Scan(&senderName, &senderAvatar); err != nil {
		return model.Favorite{}, err
	}
	if message.Reply != nil {
		if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", message.Reply.SenderID).Scan(&message.Reply.SenderName); err != nil {
			return model.Favorite{}, err
		}
	}
	payload, err := json.Marshal(message)
	if err != nil {
		return model.Favorite{}, err
	}
	ids := []string{}
	seen := map[string]bool{}
	add := func(id string) {
		if id != "" && !seen[id] {
			seen[id] = true
			ids = append(ids, id)
		}
	}
	add(message.MediaID)
	for _, b := range message.Blocks {
		add(b.MediaID)
	}
	assets := make([]favoriteAsset, 0, len(ids))
	created := []string{}
	cleanup := func() {
		for _, k := range created {
			_ = service.Files.Remove(k)
		}
	}
	for _, id := range ids {
		var a favoriteAsset
		a.mediaID = id
		a.key = model.NewID()
		var oldKey string
		err = tx.QueryRow(ctx, "SELECT file_name,media_type,content_type,byte_size,storage_key FROM message_media WHERE id=$1", id).Scan(&a.fileName, &a.mediaType, &a.contentType, &a.size, &oldKey)
		if errors.Is(err, pgx.ErrNoRows) {
			cleanup()
			return model.Favorite{}, model.Fail(404, "not_found")
		}
		if err != nil {
			cleanup()
			return model.Favorite{}, err
		}
		// Register before writing bytes; rollback/crash leaves a delayed retry.
		if _, err = service.Store.DB.Exec(ctx, `INSERT INTO favorite_asset_cleanup(storage_key,ready_at) VALUES($1,now()+interval '1 hour')`, a.key); err != nil {
			cleanup()
			return model.Favorite{}, err
		}
		created = append(created, a.key)
		in, e := service.Files.Open(oldKey)
		if e != nil {
			cleanup()
			return model.Favorite{}, e
		}
		out, e := service.Files.Create(a.key)
		var written int64
		if e == nil {
			written, e = io.Copy(out, in)
		}
		_ = in.Close()
		if out != nil {
			if closeErr := out.Close(); e == nil {
				e = closeErr
			}
		}
		if e == nil && written != a.size {
			e = io.ErrUnexpectedEOF
		}
		if e != nil {
			cleanup()
			_ = service.Files.Remove(a.key)
			return model.Favorite{}, e
		}
		assets = append(assets, a)
	}
	id := model.NewID()
	var saved time.Time
	err = tx.QueryRow(ctx, `INSERT INTO favorites(id,user_id,message_id,conversation_id,message,sender_name,sender_avatar,conversation_name,conversation_kind,is_own) VALUES($1,$2,$3,$4,$5::jsonb,$6,$7,$8,$9,$10) ON CONFLICT(user_id,message_id) DO UPDATE SET id=favorites.id RETURNING id,saved_at`, id, userID, messageID, conversationID, payload, senderName, senderAvatar, name, kind, message.SenderID == userID).Scan(&id, &saved)
	if err != nil {
		cleanup()
		return model.Favorite{}, err
	}
	for _, a := range assets {
		_, err = tx.Exec(ctx, "INSERT INTO favorite_assets(favorite_id,media_id,file_name,media_type,content_type,byte_size,storage_key) VALUES($1,$2,$3,$4,$5,$6,$7)", id, a.mediaID, a.fileName, a.mediaType, a.contentType, a.size, a.key)
		if err != nil {
			cleanup()
			return model.Favorite{}, err
		}
		if _, err = tx.Exec(ctx, "DELETE FROM favorite_asset_cleanup WHERE storage_key=$1", a.key); err != nil {
			cleanup()
			return model.Favorite{}, err
		}
	}
	if err = tx.Commit(ctx); err != nil {
		// A connection failure can leave the commit result unknown. Never
		// delete possibly committed assets; the durable queue handles rollback.
		return model.Favorite{}, err
	}
	var item model.Favorite
	item.ID = id
	item.Message = message
	item.SenderName = senderName
	item.SenderAvatar = senderAvatar
	item.ConversationName = name
	item.ConversationKind = kind
	item.SavedAt = saved
	item.IsOwn = message.SenderID == userID
	return item, nil
}

func (service *FavoriteService) List(ctx context.Context, userID, beforeID string, limit int) (model.FavoritePage, error) {
	page := model.FavoritePage{Favorites: []model.Favorite{}}
	if limit < 1 || limit > 100 {
		return page, model.Fail(400, "invalid_limit")
	}
	q := `SELECT f.id,f.message,f.sender_name,f.sender_avatar,f.conversation_name,f.conversation_kind,f.saved_at,f.is_own FROM favorites f WHERE f.user_id=$1`
	args := []any{userID}
	if beforeID != "" {
		q += ` AND (f.saved_at,f.id)<(SELECT saved_at,id FROM favorites WHERE id=$2 AND user_id=$1)`
		args = append(args, beforeID)
	}
	q += ` ORDER BY f.saved_at DESC,f.id DESC LIMIT $` + strconv.Itoa(len(args)+1)
	args = append(args, limit+1)
	rows, err := service.Store.DB.Query(ctx, q, args...)
	if err != nil {
		return page, err
	}
	defer rows.Close()
	for rows.Next() {
		var f model.Favorite
		var raw []byte
		if err = rows.Scan(&f.ID, &raw, &f.SenderName, &f.SenderAvatar, &f.ConversationName, &f.ConversationKind, &f.SavedAt, &f.IsOwn); err != nil {
			return page, err
		}
		if err = json.Unmarshal(raw, &f.Message); err != nil {
			return page, err
		}
		page.Favorites = append(page.Favorites, f)
	}
	if err = rows.Err(); err != nil {
		return page, err
	}
	if len(page.Favorites) > limit {
		page.HasMore = true
		page.Favorites = page.Favorites[:limit]
	}
	if len(page.Favorites) > 0 {
		page.NextBeforeID = page.Favorites[len(page.Favorites)-1].ID
	}
	return page, nil
}

func (service *FavoriteService) Delete(ctx context.Context, userID, id string) error {
	tag, err := service.Store.DB.Exec(ctx, "DELETE FROM favorites WHERE id=$1 AND user_id=$2", id, userID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	return service.Clean(ctx)
}
func (service *FavoriteService) Clean(ctx context.Context) error {
	rows, err := service.Store.DB.Query(ctx, "SELECT storage_key FROM favorite_asset_cleanup q WHERE ready_at<=now() AND NOT EXISTS (SELECT 1 FROM favorite_assets a WHERE a.storage_key=q.storage_key) ORDER BY created_at LIMIT 100")
	if err != nil {
		return err
	}
	defer rows.Close()
	var keys []string
	for rows.Next() {
		var k string
		if err = rows.Scan(&k); err != nil {
			return err
		}
		keys = append(keys, k)
	}
	for _, k := range keys {
		if err = service.Files.Remove(k); err != nil {
			continue
		}
		if _, err = service.Store.DB.Exec(ctx, "DELETE FROM favorite_asset_cleanup WHERE storage_key=$1", k); err != nil {
			return err
		}
	}
	return rows.Err()
}
func (service *FavoriteService) Media(ctx context.Context, userID, favoriteID, mediaID string) (model.MediaUpload, error) {
	var a model.MediaUpload
	err := service.Store.DB.QueryRow(ctx, `SELECT a.media_id,a.file_name,a.media_type,a.content_type,a.byte_size,a.storage_key,a.created_at FROM favorite_assets a JOIN favorites f ON f.id=a.favorite_id WHERE a.favorite_id=$1 AND a.media_id=$2 AND f.user_id=$3`, favoriteID, mediaID, userID).Scan(&a.ID, &a.FileName, &a.MediaType, &a.ContentType, &a.ByteSize, &a.StorageKey, &a.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return a, model.Fail(404, "not_found")
	}
	return a, err
}
func (service *FavoriteService) Open(a model.MediaUpload) (*os.File, error) {
	return service.Files.Open(a.StorageKey)
}
