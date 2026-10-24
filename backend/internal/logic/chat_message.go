package logic

import (
	"context"
	"errors"
	"github.com/jackc/pgx/v5"
	"slices"
	"strings"
	"time"
	"websocket_chat_go/backend/internal/model"
)

// MaxMessageBlocks bounds how many segments one message may interleave, matching
// the database constraint so a client gets a clear error before the insert.
const MaxMessageBlocks = 20

// maxMessageRunes is the length limit for the text a message carries, whether it
// arrives as content or as text blocks.
const maxMessageRunes = 4000

// normalizeBlocks trims the segments a client sent, drops empty ones and joins the
// text into the plain content the rest of the system reads. Image metadata is left
// for the send step to fill from the database, so a client cannot label an upload.
func normalizeBlocks(blocks []model.Block) ([]model.Block, string, error) {
	if len(blocks) > MaxMessageBlocks {
		return nil, "", model.Fail(400, "invalid_message")
	}
	normalized := make([]model.Block, 0, len(blocks))
	texts := make([]string, 0, len(blocks))
	total := 0
	for _, block := range blocks {
		switch block.Type {
		case "text":
			text := strings.TrimSpace(block.Text)
			if text == "" {
				continue
			}
			total += len([]rune(text))
			if total > maxMessageRunes {
				return nil, "", model.Fail(400, "invalid_message")
			}
			texts = append(texts, text)
			normalized = append(normalized, model.Block{Type: "text", Text: text})
		case "image":
			if block.MediaID == "" {
				return nil, "", model.Fail(400, "invalid_message")
			}
			normalized = append(normalized, model.Block{Type: "image", MediaID: block.MediaID})
		default:
			// An unknown segment cannot be rendered, so it is refused rather than
			// silently dropped.
			return nil, "", model.Fail(400, "invalid_message")
		}
	}
	if len(normalized) == 0 {
		return nil, "", model.Fail(400, "invalid_message")
	}
	return normalized, strings.Join(texts, "\n"), nil
}

// blocksSignature is the idempotency key for a message body: it ignores the media
// metadata the server fills in, so a retried send compares equal to the stored one.
func blocksSignature(blocks []model.Block) string {
	parts := make([]string, 0, len(blocks))
	for _, block := range blocks {
		if block.Type == "text" {
			parts = append(parts, "text:"+block.Text)
			continue
		}
		parts = append(parts, "image:"+block.MediaID)
	}
	return strings.Join(parts, "\n")
}

func ValidateSend(input model.SendInput) (model.SendInput, error) {
	// A message built from blocks carries its text inside them, so content is derived
	// rather than required from the caller.
	if len(input.Blocks) > 0 {
		blocks, content, err := normalizeBlocks(input.Blocks)
		if err != nil {
			return input, err
		}
		input.Blocks = blocks
		input.Content = content
		// The single-attachment field belongs to the older shape; keeping it empty
		// stops a rich message from rendering twice.
		input.MediaID = ""
	} else {
		input.Content = strings.TrimSpace(input.Content)
	}
	validContent := len([]rune(input.Content)) >= 1 && len([]rune(input.Content)) <= maxMessageRunes
	// A media message may carry an optional caption but must reference an
	// upload, so an empty caption is only acceptable together with media_id.
	if input.ConversationID == "" || len(input.ClientMsgID) < 8 || len(input.ClientMsgID) > 100 || (input.MediaID == "" && len(input.Blocks) == 0 && !validContent) || (input.MediaID != "" && len([]rune(input.Content)) > maxMessageRunes) {
		return input, model.Fail(400, "invalid_message")
	}
	if len(input.Mentions) > 100 {
		return input, model.Fail(400, "invalid_mentions")
	}
	seen := make(map[string]struct{}, len(input.Mentions))
	mentions := make([]string, 0, len(input.Mentions))
	for _, userID := range input.Mentions {
		if userID == "" {
			return input, model.Fail(400, "invalid_mentions")
		}
		if _, duplicate := seen[userID]; duplicate {
			continue
		}
		seen[userID] = struct{}{}
		mentions = append(mentions, userID)
	}
	slices.Sort(mentions)
	input.Mentions = mentions
	return input, nil
}

func (chat *Chat) Send(ctx context.Context, userID string, input model.SendInput) (model.Message, bool, error) {
	input, err := ValidateSend(input)
	if err != nil {
		return model.Message{}, false, err
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return model.Message{}, false, err
	}
	defer tx.Rollback(ctx)
	if _, err = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(hashtextextended($1, 0))", "send:"+userID); err != nil {
		return model.Message{}, false, err
	}
	var member bool
	if err = tx.QueryRow(ctx, "SELECT EXISTS(SELECT 1 FROM conversation_members WHERE conversation_id=$1 AND user_id=$2)", input.ConversationID, userID).Scan(&member); err != nil {
		return model.Message{}, false, err
	}
	if !member {
		return model.Message{}, false, model.Fail(404, "not_found")
	}
	// A muted member of a muted group, or a member under a timed mute, cannot
	// speak until an admin lifts the mute.
	var role string
	var mutedUntil *time.Time
	var allMuted, dismissed bool
	if err = tx.QueryRow(ctx, `SELECT cm.role,cm.muted_until,c.all_muted,c.dismissed_at IS NOT NULL FROM conversation_members cm JOIN conversations c ON c.id=cm.conversation_id
		WHERE cm.conversation_id=$1 AND cm.user_id=$2`, input.ConversationID, userID).Scan(&role, &mutedUntil, &allMuted, &dismissed); err != nil {
		return model.Message{}, false, err
	}
	if dismissed {
		return model.Message{}, false, model.Fail(403, "conversation_dismissed")
	}
	if mutedUntil != nil && mutedUntil.After(time.Now()) {
		return model.Message{}, false, model.Fail(403, "member_muted")
	}
	if allMuted && role == model.RoleMember {
		return model.Message{}, false, model.Fail(403, "conversation_muted")
	}
	// Blocking only silences one-to-one traffic: a group is every member's room.
	var conversationKind string
	if err = tx.QueryRow(ctx, "SELECT kind FROM conversations WHERE id=$1", input.ConversationID).Scan(&conversationKind); err != nil {
		return model.Message{}, false, err
	}
	if conversationKind == "direct" {
		var otherID string
		if err = tx.QueryRow(ctx, "SELECT user_id FROM conversation_members WHERE conversation_id=$1 AND user_id<>$2", input.ConversationID, userID).Scan(&otherID); err != nil {
			return model.Message{}, false, err
		}
		if blocked, err := blockedBetween(ctx, tx, otherID, userID); err != nil {
			return model.Message{}, false, err
		} else if blocked {
			return model.Message{}, false, model.Fail(403, "blocked_by_user")
		}
	}
	// A message reopens the conversation for everyone who had deleted it, which
	// is what turns "delete for me" into a view filter instead of a leave.
	if _, err = tx.Exec(ctx, "UPDATE conversation_members SET deleted_at=NULL WHERE conversation_id=$1 AND deleted_at IS NOT NULL", input.ConversationID); err != nil {
		return model.Message{}, false, err
	}
	kind := "user"
	if len(input.Blocks) > 0 {
		// Every image block must reference an upload that belongs to the sender in
		// this conversation, and the metadata shown to readers is taken from that
		// row rather than from the client's payload.
		for index := range input.Blocks {
			block := &input.Blocks[index]
			if block.Type != "image" {
				continue
			}
			var fileName, mediaType, contentType string
			var byteSize int64
			err = tx.QueryRow(ctx, "SELECT file_name,media_type,content_type,byte_size FROM message_media WHERE id=$1 AND owner_id=$2 AND conversation_id=$3", block.MediaID, userID, input.ConversationID).Scan(&fileName, &mediaType, &contentType, &byteSize)
			if errors.Is(err, pgx.ErrNoRows) {
				return model.Message{}, false, model.Fail(404, "not_found")
			}
			if err != nil {
				return model.Message{}, false, err
			}
			block.FileName = fileName
			block.MediaType = mediaType
			block.ContentType = contentType
			block.ByteSize = byteSize
		}
	} else if input.MediaID != "" {
		// An attachment must belong to the sender and to this conversation.
		var mediaKind string
		err = tx.QueryRow(ctx, "SELECT media_type FROM message_media WHERE id=$1 AND owner_id=$2 AND conversation_id=$3", input.MediaID, userID, input.ConversationID).Scan(&mediaKind)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Message{}, false, model.Fail(404, "not_found")
		}
		if err != nil {
			return model.Message{}, false, err
		}
		kind = mediaKind
	}
	existing, err := scanMessage(tx.QueryRow(ctx, "SELECT "+messageColumns+" FROM messages WHERE sender_id=$1 AND client_msg_id=$2", userID, input.ClientMsgID))
	if err == nil {
		if existing.ConversationID != input.ConversationID || existing.Content != input.Content || existing.MediaID != input.MediaID || existing.ReplyToID != input.ReplyToID || !slices.Equal(existing.Mentions, input.Mentions) || blocksSignature(existing.Blocks) != blocksSignature(input.Blocks) {
			return model.Message{}, false, model.Fail(409, "message_conflict")
		}
		return existing, true, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return model.Message{}, false, err
	}
	if input.ReplyToID != "" {
		// The quoted message must live in the same conversation, otherwise a
		// caller could leak context across conversations they belong to.
		// The thread root is derived here rather than trusted from the client: a
		// reply to a reply must join the original window, and a reply to a root
		// starts one. A client-supplied root would be an easy way to file a reply
		// into an unrelated window.
		var quotedRoot string
		err = tx.QueryRow(ctx, "SELECT COALESCE(thread_root_id,'') FROM messages WHERE id=$1 AND conversation_id=$2", input.ReplyToID, input.ConversationID).Scan(&quotedRoot)
		if errors.Is(err, pgx.ErrNoRows) {
			return model.Message{}, false, model.Fail(404, "not_found")
		}
		if err != nil {
			return model.Message{}, false, err
		}
		if quotedRoot != "" {
			// Replying inside a window joins that window.
			input.ThreadRootID = quotedRoot
		} else {
			// Replying to a root message starts a window rooted at it.
			input.ThreadRootID = input.ReplyToID
		}
	} else {
		input.ThreadRootID = ""
	}
	if len(input.Mentions) > 0 {
		// Mentioned users must actually be members at send time.
		var count int
		if err = tx.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND user_id=ANY($2)", input.ConversationID, input.Mentions).Scan(&count); err != nil {
			return model.Message{}, false, err
		}
		if count != len(input.Mentions) {
			return model.Message{}, false, model.Fail(400, "invalid_mentions")
		}
	}
	var seq int64
	if err = tx.QueryRow(ctx, "UPDATE conversations SET next_seq=next_seq+1 WHERE id=$1 RETURNING next_seq", input.ConversationID).Scan(&seq); err != nil {
		return model.Message{}, false, err
	}
	// ThreadRootID must be passed through: it is what files the reply into a
	// window, and omitting it here would silently create orphan replies. Blocks
	// travel with it so the reader keeps the order the sender wrote.
	message, err := insertMessage(ctx, tx, model.Message{ConversationID: input.ConversationID, SenderID: userID, ClientMsgID: input.ClientMsgID, Seq: seq, Content: input.Content, Kind: kind, MediaID: input.MediaID, Blocks: input.Blocks, ReplyToID: input.ReplyToID, ThreadRootID: input.ThreadRootID, Mentions: input.Mentions})
	if err != nil {
		return model.Message{}, false, err
	}
	if err = chat.appendEvent(ctx, tx, input.ConversationID, model.Event{Event: "message.created", Data: message}); err != nil {
		return model.Message{}, false, err
	}
	if err = tx.Commit(ctx); err != nil {
		return model.Message{}, false, err
	}
	return message, false, nil
}

// Poke adds a playful "拍一拍" system notice aimed at one member. Both sides
// must belong to the conversation, and the notice roams like every other
// system message.
func (chat *Chat) Poke(ctx context.Context, userID, conversationID, targetID string) error {
	if targetID == "" {
		return model.Fail(400, "invalid_members")
	}
	tx, err := chat.Store.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var count int
	if err = tx.QueryRow(ctx, `SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND user_id=ANY($2)`, conversationID, []string{userID, targetID}).Scan(&count); err != nil {
		return err
	}
	if count != 2 {
		return model.Fail(404, "not_found")
	}
	var callerName, targetName string
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", userID).Scan(&callerName); err != nil {
		return err
	}
	if err = tx.QueryRow(ctx, "SELECT name FROM users WHERE id=$1", targetID).Scan(&targetName); err != nil {
		return err
	}
	if err = chat.systemNotice(ctx, tx, conversationID, userID, "「"+callerName+"」拍了拍「"+targetName+"」"); err != nil {
		return err
	}
	return tx.Commit(ctx)
}
