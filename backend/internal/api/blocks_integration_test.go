package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

// uploadImage stores one small PNG for the caller and returns its id.
func uploadImage(t *testing.T, server *httptest.Server, token, conversationID string) string {
	t.Helper()
	uploaded, _ := uploadMedia(t, server, token, conversationID, "shot.png", "image/png", tinyPNG(), 201)
	if uploaded.ID == "" {
		t.Fatal("upload returned no id")
	}
	return uploaded.ID
}

// tinyPNG is a 1x1 PNG: enough for the upload endpoint, which validates the type
// it is told rather than the bytes.
func tinyPNG() []byte {
	return []byte{
		0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A,
		0x00, 0x00, 0x00, 0x0D, 0x49, 0x48, 0x44, 0x52,
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x06, 0x00, 0x00, 0x00, 0x1F, 0x15, 0xC4,
		0x89, 0x00, 0x00, 0x00, 0x0A, 0x49, 0x44, 0x41,
		0x54, 0x78, 0x9C, 0x63, 0x00, 0x01, 0x00, 0x00,
		0x05, 0x00, 0x01, 0x0D, 0x0A, 0x2D, 0xB4, 0x00,
		0x00, 0x00, 0x00, 0x49, 0x45, 0x4E, 0x44, 0xAE,
		0x42, 0x60, 0x82,
	}
}

// TestMessageBlocksKeepTheTypedOrder is the behaviour the composer relies on: one
// message carries interleaved text and images in the order they were written.
func TestMessageBlocksKeepTheTypedOrder(t *testing.T) {
	_, server, _, aliceToken, _, _, _, _, group := groupFixture(t)

	first := uploadImage(t, server, aliceToken, group)
	second := uploadImage(t, server, aliceToken, group)

	blocks := []model.Block{
		{Type: "text", Text: "先看这张"},
		{Type: "image", MediaID: first},
		{Type: "text", Text: "再看下面这张"},
		{Type: "image", MediaID: second},
		{Type: "text", Text: "两者对比即可"},
	}
	body := map[string]any{"client_msg_id": "blocks-order-0001", "blocks": blocks}
	raw := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", body, 201)
	var message model.Message
	if err := json.Unmarshal(raw, &message); err != nil {
		t.Fatal(err)
	}

	if len(message.Blocks) != 5 {
		t.Fatalf("expected five blocks, got %d: %+v", len(message.Blocks), message.Blocks)
	}
	wantTypes := []string{"text", "image", "text", "image", "text"}
	for index, want := range wantTypes {
		if message.Blocks[index].Type != want {
			t.Fatalf("block %d is %q, want %q", index, message.Blocks[index].Type, want)
		}
	}
	// The images keep the order they were inserted in, not the upload order.
	if message.Blocks[1].MediaID != first || message.Blocks[3].MediaID != second {
		t.Fatalf("image order changed: %q then %q", message.Blocks[1].MediaID, message.Blocks[3].MediaID)
	}
	// The server fills the media metadata, so a reader renders without a second call.
	if message.Blocks[1].FileName == "" || message.Blocks[1].MediaType != "image" || message.Blocks[1].ByteSize == 0 {
		t.Fatalf("image metadata was not filled: %+v", message.Blocks[1])
	}
	// The plain content is the text segments, so search and reply excerpts work.
	if message.Content != "先看这张\n再看下面这张\n两者对比即可" {
		t.Fatalf("content is not the joined text: %q", message.Content)
	}
	// A rich message must not also set the single-attachment field, or it renders twice.
	if message.MediaID != "" {
		t.Fatalf("a blocks message must not carry media_id: %q", message.MediaID)
	}

	// The order survives a read from the history endpoint.
	history := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages?before_seq=99999&limit=10", nil, 200)
	var page []model.Message
	if err := json.Unmarshal(history, &page); err != nil {
		t.Fatal(err)
	}
	var stored *model.Message
	for index := range page {
		if page[index].ID == message.ID {
			stored = &page[index]
		}
	}
	if stored == nil {
		t.Fatal("the message was not returned by history")
	}
	if len(stored.Blocks) != 5 || stored.Blocks[1].MediaID != first || stored.Blocks[3].MediaID != second {
		t.Fatalf("the stored order differs from what was sent: %+v", stored.Blocks)
	}
}

// A client must not be able to label an upload or reference someone else's.
func TestMessageBlocksAreValidated(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, _, group := groupFixture(t)

	aliceMedia := uploadImage(t, server, aliceToken, group)
	// Bob's upload belongs to Bob, so Alice cannot embed it.
	bobMedia := uploadImage(t, server, bobToken, group)

	// An empty segment list, an unknown segment and an empty image reference are
	// all refused rather than silently dropped.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-empty-0001", "blocks": []model.Block{}}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-kind-0001", "blocks": []model.Block{{Type: "video", MediaID: aliceMedia}}}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-noimg-0001", "blocks": []model.Block{{Type: "image"}}}, 400)
	// Whitespace-only text is dropped, so this leaves nothing to send.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-blank-0001", "blocks": []model.Block{{Type: "text", Text: "   "}}}, 400)
	// Too many segments are refused before the database constraint sees them.
	many := make([]model.Block, 0, 21)
	for index := 0; index < 21; index++ {
		many = append(many, model.Block{Type: "text", Text: "x"})
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-many-0001", "blocks": many}, 400)
	// Someone else's upload is not found.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-bob-0001", "blocks": []model.Block{{Type: "image", MediaID: bobMedia}}}, 404)

	// A single image block with no text is a valid message.
	single := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{"client_msg_id": "blocks-single-0001", "blocks": []model.Block{{Type: "image", MediaID: aliceMedia}}}, 201)
	var message model.Message
	if err := json.Unmarshal(single, &message); err != nil {
		t.Fatal(err)
	}
	if len(message.Blocks) != 1 || message.Blocks[0].MediaID != aliceMedia {
		t.Fatalf("a single image message was not stored: %+v", message.Blocks)
	}

	// A client-supplied filename must be replaced by the stored one.
	forged := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages", map[string]any{
		"client_msg_id": "blocks-forged-001",
		"blocks":        []model.Block{{Type: "image", MediaID: aliceMedia, FileName: "forged.exe", ContentType: "application/x-msdownload"}},
	}, 201)
	var forgedMessage model.Message
	if err := json.Unmarshal(forged, &forgedMessage); err != nil {
		t.Fatal(err)
	}
	if forgedMessage.Blocks[0].FileName == "forged.exe" || forgedMessage.Blocks[0].ContentType == "application/x-msdownload" {
		t.Fatalf("the client labelled the upload: %+v", forgedMessage.Blocks[0])
	}
}
