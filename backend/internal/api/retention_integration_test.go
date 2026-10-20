package api

import (
	"encoding/json"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/media"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
	"websocket_chat_go/backend/internal/svc"
)

// TestRetentionPurgesOnlyDisposableData checks both halves of the cleanup: it
// reclaims published outbox rows and withdrawn attachment bytes, while leaving
// unpublished rows and live attachments untouched.
func TestRetentionPurgesOnlyDisposableData(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "retention-alice")
	bob, bobToken := account(t, services, "retention-bob")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "retention", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	// A live attachment and a withdrawn one, each with its own message.
	liveUpload, _ := uploadMedia(t, server, aliceToken, group, "live.png", "image/png", []byte("live bytes"), 201)
	recalledUpload, _ := uploadMedia(t, server, aliceToken, group, "gone.png", "image/png", []byte("gone bytes"), 201)
	live := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "retention-live-1", "content": "", "media_id": liveUpload.ID}, 201)
	recalled := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "retention-gone-1", "content": "", "media_id": recalledUpload.ID}, 201)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(recalled.Seq)+"/recall", nil, 200)

	// This fixture does not start the dispatcher, so no row is published yet.
	// Mark all but one as published-and-aged, which is exactly the state the
	// retention worker is meant to reclaim.
	if _, err := services.Store.DB.Exec(t.Context(), `UPDATE message_outbox SET published_at=now()-interval '30 days'
		WHERE id <> (SELECT max(id) FROM message_outbox)`); err != nil {
		t.Fatal(err)
	}
	var published, unpublished int
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FILTER (WHERE published_at IS NOT NULL), count(*) FILTER (WHERE published_at IS NULL) FROM message_outbox").Scan(&published, &unpublished); err != nil {
		t.Fatal(err)
	}
	if published == 0 || unpublished == 0 {
		t.Fatalf("fixture needs both published and unpublished rows: %d/%d", published, unpublished)
	}

	retention := &realtime.Retention{Store: services.Store, Files: services.Media.Files, OutboxAge: 7 * 24 * time.Hour}
	rows, files, err := retention.Sweep(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if rows != int64(published) {
		t.Fatalf("expected %d published rows removed, got %d", published, rows)
	}
	if files != 1 {
		t.Fatalf("expected exactly the withdrawn attachment to be reclaimed, got %d", files)
	}

	// The unpublished row survives, and so do the live attachment's bytes.
	var remaining int
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM message_outbox").Scan(&remaining); err != nil || remaining != unpublished {
		t.Fatalf("unpublished outbox rows were removed: %d %v", remaining, err)
	}
	download(t, server, bobToken, liveUpload.ID, 200)
	// The withdrawn attachment is already unreachable by API; its row is marked.
	var marked bool
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT bytes_removed_at IS NOT NULL FROM message_media WHERE id=$1", recalledUpload.ID).Scan(&marked); err != nil || !marked {
		t.Fatalf("withdrawn attachment was not marked removed: %v %v", marked, err)
	}
	// The status column for the recalled message is still there, so history keeps
	// a record of what was withdrawn rather than losing the position.
	var kind string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT kind FROM messages WHERE id=$1", recalled.ID).Scan(&kind); err != nil || kind != "image" {
		t.Fatalf("withdrawing the bytes should not delete the message row: %q %v", kind, err)
	}
	_ = live
}

// TestRetentionIsIdempotent confirms a second sweep reclaims nothing and does not
// error, which matters because the worker runs on a timer.
func TestRetentionIsIdempotent(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "retention-idem")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "idem"})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, _ := uploadMedia(t, server, aliceToken, group, "once.png", "image/png", []byte("once"), 201)
	message := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "retention-once-1", "content": "", "media_id": uploaded.ID}, 201)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(message.Seq)+"/recall", nil, 200)

	retention := &realtime.Retention{Store: services.Store, Files: services.Media.Files}
	if _, files, err := retention.Sweep(t.Context()); err != nil || files != 1 {
		t.Fatalf("first sweep: files=%d err=%v", files, err)
	}
	if _, files, err := retention.Sweep(t.Context()); err != nil || files != 0 {
		t.Fatalf("second sweep should reclaim nothing: files=%d err=%v", files, err)
	}
}

// TestRetentionKeepsBytesWhenMediaIsLive is the safety half: an attachment that
// no recall references must never be deleted by the sweep.
func TestRetentionKeepsBytesWhenMediaIsLive(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "retention-live-keep")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "keep"})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, _ := uploadMedia(t, server, aliceToken, group, "keep.png", "image/png", []byte("keep"), 201)
	sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "retention-keep-1", "content": "", "media_id": uploaded.ID}, 201)

	retention := &realtime.Retention{Store: services.Store, Files: services.Media.Files}
	if _, files, err := retention.Sweep(t.Context()); err != nil || files != 0 {
		t.Fatalf("a live attachment must not be reclaimed: files=%d err=%v", files, err)
	}
	// The file is still on disk, not merely still referenced.
	var storageKey string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM message_media WHERE id=$1", uploaded.ID).Scan(&storageKey); err != nil {
		t.Fatal(err)
	}
	files, err := media.NewStore(filepath.Join(services.Config.MediaDir))
	if err != nil {
		t.Fatal(err)
	}
	file, err := files.Open(storageKey)
	if err != nil {
		t.Fatalf("live attachment bytes were deleted: %v", err)
	}
	file.Close()
}

// Keep the unused imports honest: logic and svc are used by the helpers above.
var (
	_ = logic.Credentials{}
	_ = svc.ServiceContext{}
	_ = json.Marshal
)
