package logic

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"websocket_chat_go/backend/internal/model"
)

const favoriteColumns = "id,message,sender_name,sender_avatar,conversation_name,conversation_kind,saved_at,is_own,tags,note"

func scanFavorite(row pgx.Row) (f model.Favorite, err error) {
	var raw []byte
	err = row.Scan(&f.ID, &raw, &f.SenderName, &f.SenderAvatar, &f.ConversationName, &f.ConversationKind, &f.SavedAt, &f.IsOwn, &f.Tags, &f.Note)
	if err == nil {
		err = json.Unmarshal(raw, &f.Message)
	}
	return
}

func (s *FavoriteService) Patch(ctx context.Context, userID, id string, tags *[]string, note *string) (model.Favorite, error) {
	var clean []string
	if tags != nil {
		clean = []string{}
		seen := map[string]bool{}
		for _, value := range *tags {
			value = strings.TrimSpace(value)
			if !utf8.ValidString(value) || utf8.RuneCountInString(value) > 20 {
				return model.Favorite{}, model.Fail(400, "invalid_tags")
			}
			if value == "" || seen[value] {
				continue
			}
			clean = append(clean, value)
			seen[value] = true
		}
		if len(clean) > 10 {
			return model.Favorite{}, model.Fail(400, "invalid_tags")
		}
	}
	if note != nil && (!utf8.ValidString(*note) || utf8.RuneCountInString(*note) > 2000) {
		return model.Favorite{}, model.Fail(400, "invalid_note")
	}
	var tagArg any
	var noteArg any
	if tags != nil {
		tagArg = clean
	}
	if note != nil {
		noteArg = *note
	}
	item, err := scanFavorite(s.Store.DB.QueryRow(ctx, "UPDATE favorites SET tags=CASE WHEN $3::boolean THEN $4::text[] ELSE tags END,note=CASE WHEN $5::boolean THEN $6::text ELSE note END WHERE id=$1 AND user_id=$2 RETURNING "+favoriteColumns, id, userID, tags != nil, tagArg, note != nil, noteArg))
	if errors.Is(err, pgx.ErrNoRows) {
		return model.Favorite{}, model.Fail(404, "not_found")
	}
	return item, err
}
