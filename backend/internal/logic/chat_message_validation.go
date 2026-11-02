package logic

import (
	"slices"
	"strings"

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
