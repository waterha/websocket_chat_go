package api

import (
	"bytes"
	"context"
	"encoding/json"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"sync"
	"testing"
	"time"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

func saveSticker(t *testing.T, s *httptest.Server, token, cid, mid, mediaID string) model.Sticker {
	t.Helper()
	var item model.Sticker
	raw := httpCall(t, s, token, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": mid, "media_id": mediaID}, 201)
	if err := json.Unmarshal(raw, &item); err != nil {
		t.Fatal(err)
	}
	return item
}
func stickerDownload(t *testing.T, s *httptest.Server, token, id string, want int) []byte {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), "GET", s.URL+"/api/v1/stickers/"+id+"/media", nil)
	req.Header.Set("Cookie", "session="+token)
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("sticker media %d want %d: %s", resp.StatusCode, want, b)
	}
	return b
}
func TestStickersLimitsConcurrencyAndRecall(t *testing.T) {
	services := fixture(t, false)
	alice, at := account(t, services, "sticker-race")
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "race"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := uploadMedia(t, s, at, cid, "a.png", "image/png", []byte("source"), 201)
	b, _ := uploadMedia(t, s, at, cid, "b.png", "image/png", []byte("second"), 201)
	m, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "sticker-multi", Blocks: []model.Block{{Type: "image", MediaID: a.ID}, {Type: "image", MediaID: b.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, at, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": m.ID}, 400)
	poolConfig := services.Store.DB.Config()
	poolConfig.MaxConns = 2
	smallPool, err := pgxpool.NewWithConfig(t.Context(), poolConfig)
	if err != nil {
		t.Fatal(err)
	}
	oldPool := services.Store.DB
	services.Store.DB = smallPool
	oldPool.Close()
	concurrentCtx, cancelConcurrent := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancelConcurrent()
	var wg sync.WaitGroup
	results := make(chan model.Sticker, 6)
	failures := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, e := services.Stickers.Create(concurrentCtx, alice.ID, cid, m.ID, a.ID)
			if e != nil {
				failures <- e
				return
			}
			results <- item
		}()
	}
	wg.Wait()
	close(results)
	close(failures)
	for e := range failures {
		t.Fatal(e)
	}
	var first model.Sticker
	for item := range results {
		if first.ID == "" {
			first = item
		}
		if first != item {
			t.Fatal("concurrent dedup changed identity")
		}
	}
	prepareErrors := make(chan error, 6)
	for i := 0; i < 6; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := services.Stickers.Prepare(concurrentCtx, alice.ID, first.ID, cid)
			prepareErrors <- e
		}()
	}
	wg.Wait()
	close(prepareErrors)
	for e := range prepareErrors {
		if e != nil {
			t.Fatal(e)
		}
	}
	// Fill to the cap cheaply while retaining the same real source API check.
	if _, err = services.Store.DB.Exec(t.Context(), `INSERT INTO stickers(id,user_id,kind,source_key,text) SELECT 'fill_'||n,$1,'emoji','fill_'||n,'😀' FROM generate_series(1,199) n`, alice.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Stickers.Create(t.Context(), alice.ID, cid, m.ID, b.ID); err == nil {
		t.Fatal("cap exceeded")
	} else {
		status(t, err, 409)
	}
	if duplicate, e := services.Stickers.Create(t.Context(), alice.ID, cid, m.ID, a.ID); e != nil || duplicate.ID != first.ID {
		t.Fatalf("duplicate at cap: %+v %v", duplicate, e)
	}
	if _, err = services.Store.DB.Exec(t.Context(), "DELETE FROM stickers WHERE id LIKE 'fill_%'"); err != nil {
		t.Fatal(err)
	}
	tx, err := services.Store.DB.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(t.Context(), "SELECT id FROM conversations WHERE id=$1 FOR UPDATE", cid); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { _, e := services.Stickers.Create(t.Context(), alice.ID, cid, m.ID, b.ID); done <- e }()
	if _, err = tx.Exec(t.Context(), "UPDATE messages SET recalled_at=now() WHERE id=$1", m.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-done:
		status(t, err, 400)
	case <-time.After(5 * time.Second):
		t.Fatal("recall/create deadlock")
	}
	// A queued live key must never be removed by the shared favorite cleaner.
	var key string
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM stickers WHERE id=$1", first.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Store.DB.Exec(t.Context(), "INSERT INTO favorite_asset_cleanup(storage_key) VALUES($1)", key); err != nil {
		t.Fatal(err)
	}
	if err = services.Favorites.Clean(t.Context()); err != nil {
		t.Fatal(err)
	}
	stickerDownload(t, s, at, first.ID, 200)
}

func TestStickersPrivateSnapshotPrepareAndCleanup(t *testing.T) {
	services := fixture(t, false)
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	alice, at := account(t, services, "sticker-alice")
	bob, bt := account(t, services, "sticker-bob")
	_, ot := account(t, services, "sticker-other")
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "stickers", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	data := []byte("independent sticker bytes")
	upload, _ := uploadMedia(t, s, at, cid, "sticker.png", "image/png", data, 201)
	unused, _ := uploadMedia(t, s, at, cid, "unused.png", "image/png", data, 201)
	msg, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "sticker-source", Blocks: []model.Block{{Type: "text", Text: "image"}, {Type: "image", MediaID: upload.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	item := saveSticker(t, s, bt, cid, msg.ID, upload.ID)
	if item.Kind != "image" || item.FileName != "sticker.png" || item.ByteSize != int64(len(data)) || item.Text != "" {
		t.Fatalf("bad item: %+v", item)
	}
	duplicate := saveSticker(t, s, bt, cid, msg.ID, upload.ID)
	if duplicate != item {
		t.Fatal("not idempotent")
	}
	own := saveSticker(t, s, at, cid, msg.ID, upload.ID)
	if own.ID == item.ID {
		t.Fatal("shared sticker ownership")
	}
	fav := saveFavorite(t, s, bt, cid, msg.ID)
	httpCall(t, s, ot, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": msg.ID, "media_id": upload.ID}, 404)
	httpCall(t, s, bt, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": msg.ID, "media_id": unused.ID}, 400)
	httpCall(t, s, bt, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": msg.ID, "url": "https://invalid"}, 400)
	stickerDownload(t, s, at, item.ID, 404)
	httpCall(t, s, at, "DELETE", "/api/v1/stickers/"+item.ID, nil, 404)
	httpCall(t, s, at, "POST", "/api/v1/stickers/"+item.ID+"/prepare", map[string]string{"conversation_id": cid}, 404)
	if _, err = services.Chat.Recall(t.Context(), alice.ID, cid, msg.Seq); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, bt, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": msg.ID, "media_id": upload.ID}, 400)
	if _, _, err = (&realtime.Retention{Store: services.Store, Files: services.Media.Files}).Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, bt, "POST", "/api/v1/conversations/"+cid+"/members/leave", nil, 200)
	if !bytes.Equal(stickerDownload(t, s, bt, item.ID, 200), data) {
		t.Fatal("sticker lost after recall/leave")
	}
	httpCall(t, s, bt, "POST", "/api/v1/stickers/"+item.ID+"/prepare", map[string]string{"conversation_id": cid}, 404)
	target, err := services.Chat.Create(t.Context(), bob.ID, model.CreateInput{Kind: "group", Name: "target", MemberIDs: []string{alice.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var prepared model.Media
	raw := httpCall(t, s, bt, "POST", "/api/v1/stickers/"+item.ID+"/prepare", map[string]string{"conversation_id": target}, 201)
	if err = json.Unmarshal(raw, &prepared); err != nil {
		t.Fatal(err)
	}
	if prepared.MediaType != "image" || prepared.ID == upload.ID {
		t.Fatalf("invalid prepared: %+v", prepared)
	}
	if _, _, err = services.Chat.Send(t.Context(), bob.ID, model.SendInput{ConversationID: target, ClientMsgID: "sticker-send", MediaID: prepared.ID}); err != nil {
		t.Fatal(err)
	}
	var key string
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM stickers WHERE id=$1", item.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	req, _ := http.NewRequestWithContext(t.Context(), "DELETE", s.URL+"/api/v1/stickers/"+item.ID, nil)
	req.Header.Set("Cookie", "session="+bt)
	csrf, _ := testCSRFTokens.Load(bt)
	req.Header.Set("X-CSRF-Token", csrf.(string))
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete status %d", resp.StatusCode)
	}
	stickerDownload(t, s, bt, item.ID, 404)
	if f, e := services.Media.Files.Open(key); !os.IsNotExist(e) {
		if f != nil {
			f.Close()
		}
		t.Fatalf("sticker bytes not removed: %v", e)
	}
	got, _ := favoriteDownload(t, s, bt, fav.ID, upload.ID, 200)
	if !bytes.Equal(got, data) {
		t.Fatal("sticker cleanup damaged favorite")
	}
	media, err := services.Media.Retrieve(t.Context(), bob.ID, prepared.ID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := services.Media.Open(media)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	got, err = io.ReadAll(f)
	if err != nil || !bytes.Equal(got, data) {
		t.Fatal("prepared bytes damaged by deletion")
	}
	var page model.StickerPage
	if err = json.Unmarshal(httpCall(t, s, bt, "GET", "/api/v1/stickers", nil, 200), &page); err != nil || len(page.Stickers) != 0 {
		t.Fatalf("library not isolated: %+v %v", page, err)
	}
}

func TestStickersEmojiValidationDedupAndPreparePermissions(t *testing.T) {
	services := fixture(t, false)
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	alice, at := account(t, services, "emoji-alice")
	bob, bt := account(t, services, "emoji-bob")
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "emoji", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	send := func(text string) model.Message {
		t.Helper()
		m, _, e := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: model.NewID(), Content: text})
		if e != nil {
			t.Fatal(e)
		}
		return m
	}
	first := send("👩🏽‍💻 ❤️\n😀1️⃣©️")
	item := saveSticker(t, s, bt, cid, first.ID, "")
	second := send(first.Content)
	dup := saveSticker(t, s, bt, cid, second.ID, "")
	if item.Kind != "emoji" || item.Text != first.Content || item.ID != dup.ID || item.ByteSize != 0 {
		t.Fatalf("emoji mismatch: %+v %+v", item, dup)
	}
	for _, text := range []string{"arbitrary text", "<img src=x>", "https://invalid", "😀hi", "😀‍"} {
		m := send(text)
		httpCall(t, s, bt, "POST", "/api/v1/stickers", map[string]string{"conversation_id": cid, "message_id": m.ID}, 400)
	}
	stickerDownload(t, s, bt, item.ID, 404)
	httpCall(t, s, bt, "POST", "/api/v1/stickers/"+item.ID+"/prepare", map[string]string{"conversation_id": cid}, 404)
	data := []byte("permission image")
	a, _ := uploadMedia(t, s, at, cid, "a.png", "image/png", data, 201)
	m, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "permission-image", MediaID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	image := saveSticker(t, s, bt, cid, m.ID, "")
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE conversation_members SET muted_until=now()+interval '1 hour' WHERE conversation_id=$1 AND user_id=$2", cid, bob.ID); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, bt, "POST", "/api/v1/stickers/"+image.ID+"/prepare", map[string]string{"conversation_id": cid}, 403)
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE conversation_members SET muted_until=NULL WHERE conversation_id=$1 AND user_id=$2", cid, bob.ID); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE conversations SET all_muted=true WHERE id=$1", cid); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, bt, "POST", "/api/v1/stickers/"+image.ID+"/prepare", map[string]string{"conversation_id": cid}, 403)
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE conversations SET all_muted=false,dismissed_at=now() WHERE id=$1", cid); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, bt, "POST", "/api/v1/stickers/"+image.ID+"/prepare", map[string]string{"conversation_id": cid}, 403)
}
