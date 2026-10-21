package logic

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

// MaxAnnouncements bounds how many notices one group may hold, so the panel
// cannot grow without limit and the list stays readable.
const MaxAnnouncements = 100

// AnnouncementColumns is shared by every read path so the shape stays identical.
const announcementColumns = `id,conversation_id,author_id,content,created_at,updated_at,(updated_at > created_at)`

func scanAnnouncement(row pgx.Row) (model.Announcement, error) {
	var item model.Announcement
	err := row.Scan(&item.ID, &item.ConversationID, &item.AuthorID, &item.Content, &item.CreatedAt, &item.UpdatedAt, &item.Edited)
	return item, err
}

// Announcements lists every notice of a conversation, newest first. Any member
// may read them; only owners and admins may change them.
func (chat *Chat) Announcements(ctx context.Context, userID, conversationID string) ([]model.Announcement, error) {
	if _, err := chat.ConversationSeq(ctx, userID, conversationID); err != nil {
		return nil, err
	}
	rows, err := chat.Store.DB.Query(ctx, "SELECT "+announcementColumns+" FROM announcements WHERE conversation_id=$1 ORDER BY created_at DESC,id DESC", conversationID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	items := []model.Announcement{}
	for rows.Next() {
		item, scanErr := scanAnnouncement(rows)
		if scanErr != nil {
			return nil, scanErr
		}
		items = append(items, item)
	}
	return items, rows.Err()
}

// validateAnnouncement trims the text and rejects empty or oversized notices.
func validateAnnouncement(content string) (string, error) {
	content = strings.TrimSpace(content)
	if length := len([]rune(content)); length < 1 || length > 2000 {
		return "", model.Fail(400, "invalid_announcement")
	}
	return content, nil
}

// CreateAnnouncement publishes a new notice. The author is the caller, never a
// client-supplied value.
func (chat *Chat) CreateAnnouncement(ctx context.Context, userID, conversationID, content string) (model.Announcement, error) {
	content, err := validateAnnouncement(content)
	if err != nil {
		return model.Announcement{}, err
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.Announcement{}, err
	}
	defer tx.Rollback(ctx)
	role, err := chat.roleOf(ctx, tx, userID, conversationID)
	if err != nil {
		return model.Announcement{}, err
	}
	if role != model.RoleOwner && role != model.RoleAdmin {
		return model.Announcement{}, model.Fail(403, "forbidden")
	}
	var count int
	if err = tx.QueryRow(ctx, "SELECT count(*) FROM announcements WHERE conversation_id=$1", conversationID).Scan(&count); err != nil {
		return model.Announcement{}, err
	}
	if count >= MaxAnnouncements {
		return model.Announcement{}, model.Fail(400, "too_many_announcements")
	}
	item, err := scanAnnouncement(tx.QueryRow(ctx, `INSERT INTO announcements(id,conversation_id,author_id,content)
		VALUES($1,$2,$3,$4) RETURNING `+announcementColumns, model.NewID(), conversationID, userID, content))
	if err != nil {
		return model.Announcement{}, err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return model.Announcement{}, err
	}
	return item, tx.Commit(ctx)
}

// UpdateAnnouncement edits one notice. Only owners and admins may edit, and the
// author identity is left untouched so the list keeps its provenance.
func (chat *Chat) UpdateAnnouncement(ctx context.Context, userID, conversationID, announcementID, content string) (model.Announcement, error) {
	content, err := validateAnnouncement(content)
	if err != nil {
		return model.Announcement{}, err
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.Announcement{}, err
	}
	defer tx.Rollback(ctx)
	role, err := chat.roleOf(ctx, tx, userID, conversationID)
	if err != nil {
		return model.Announcement{}, err
	}
	if role != model.RoleOwner && role != model.RoleAdmin {
		return model.Announcement{}, model.Fail(403, "forbidden")
	}
	item, err := scanAnnouncement(tx.QueryRow(ctx, `UPDATE announcements SET content=$1,updated_at=now()
		WHERE id=$2 AND conversation_id=$3 RETURNING `+announcementColumns, content, announcementID, conversationID))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Announcement{}, model.Fail(404, "not_found")
	}
	if err != nil {
		return model.Announcement{}, err
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return model.Announcement{}, err
	}
	return item, tx.Commit(ctx)
}

// DeleteAnnouncement removes one notice. Only owners and admins may delete.
func (chat *Chat) DeleteAnnouncement(ctx context.Context, userID, conversationID, announcementID string) error {
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	role, err := chat.roleOf(ctx, tx, userID, conversationID)
	if err != nil {
		return err
	}
	if role != model.RoleOwner && role != model.RoleAdmin {
		return model.Fail(403, "forbidden")
	}
	tag, err := tx.Exec(ctx, "DELETE FROM announcements WHERE id=$1 AND conversation_id=$2", announcementID, conversationID)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return model.Fail(404, "not_found")
	}
	if err = chat.notifyConversation(ctx, tx, conversationID); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

// announcementEdited reports whether a notice changed after publication, using
// the row timestamps rather than trusting the client.
func announcementEdited(created, updated time.Time) bool { return updated.After(created) }
