package api

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

func favoriteDownload(t *testing.T, s *httptest.Server, token, id, mediaID string, want int) ([]byte, http.Header) {
	t.Helper()
	r, _ := http.NewRequestWithContext(t.Context(), "GET", s.URL+"/api/v1/favorites/"+id+"/media/"+mediaID, nil)
	r.Header.Set("Cookie", "session="+token)
	resp, err := s.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("favorite download %d want %d: %s", resp.StatusCode, want, b)
	}
	return b, resp.Header
}
func saveFavorite(t *testing.T, s *httptest.Server, token, cid, mid string) model.Favorite {
	t.Helper()
	var f model.Favorite
	raw := httpCall(t, s, token, "POST", "/api/v1/favorites", map[string]string{"conversation_id": cid, "message_id": mid}, 201)
	if err := json.Unmarshal(raw, &f); err != nil {
		t.Fatal(err)
	}
	return f
}

func TestFavoritesSnapshotIsolationAndLifetime(t *testing.T) {
	services := fixture(t, false)
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	alice, at := account(t, services, "favorite-alice")
	bob, bt := account(t, services, "favorite-bob")
	_, ot := account(t, services, "favorite-outsider")
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "original group", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	quote, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "favorite-quote", Content: "引用\n第二行"})
	if err != nil {
		t.Fatal(err)
	}
	image := []byte("image bytes retained independently")
	a, _ := uploadMedia(t, s, at, cid, "a.png", "image/png", image, 201)
	msg, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "favorite-blocks", ReplyToID: quote.ID, Blocks: []model.Block{{Type: "text", Text: "第一行\n第二行"}, {Type: "image", MediaID: a.ID}, {Type: "text", Text: "末段"}, {Type: "image", MediaID: a.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	f := saveFavorite(t, s, bt, cid, msg.ID)
	if f.IsOwn || f.SenderName != alice.Name || f.ConversationName != "original group" || !reflect.DeepEqual(f.Message.Blocks, msg.Blocks) || f.Message.Content != msg.Content || !f.Message.CreatedAt.Equal(msg.CreatedAt) || f.Message.Reply == nil || f.Message.Reply.SenderName != alice.Name {
		t.Fatalf("snapshot mismatch: %+v", f)
	}
	dup := saveFavorite(t, s, bt, cid, msg.ID)
	if f.ID != dup.ID || !f.SavedAt.Equal(dup.SavedAt) {
		t.Fatal("duplicate changed identity/time")
	}
	own := saveFavorite(t, s, at, cid, msg.ID)
	if !own.IsOwn || own.ID == f.ID {
		t.Fatal("ownership not captured")
	}
	var n int
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM favorite_assets WHERE favorite_id=$1", f.ID).Scan(&n); err != nil || n != 1 {
		t.Fatalf("dedup assets: %d %v", n, err)
	}
	httpCall(t, s, ot, "POST", "/api/v1/favorites", map[string]string{"conversation_id": cid, "message_id": msg.ID}, 404)
	httpCall(t, s, bt, "POST", "/api/v1/favorites", map[string]string{"conversation_id": cid, "message_id": msg.ID, "html": "<b>forged</b>"}, 400)
	favoriteDownload(t, s, at, f.ID, a.ID, 404)
	favoriteDownload(t, s, bt, f.ID, model.NewID(), 404)
	httpCall(t, s, at, "DELETE", "/api/v1/favorites/"+f.ID, nil, 404)
	if _, err = services.Chat.Recall(t.Context(), alice.ID, cid, msg.Seq); err != nil {
		t.Fatal(err)
	}
	if _, _, err = (&realtime.Retention{Store: services.Store, Files: services.Media.Files}).Sweep(t.Context()); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, bt, "POST", "/api/v1/conversations/"+cid+"/members/leave", nil, 200)
	got, h := favoriteDownload(t, s, bt, f.ID, a.ID, 200)
	if !bytes.Equal(got, image) || h.Get("X-Content-Type-Options") != "nosniff" {
		t.Fatal("saved media lost")
	}
	page, err := services.Favorites.List(t.Context(), bob.ID, "", 30)
	if err != nil || len(page.Favorites) != 1 || !reflect.DeepEqual(page.Favorites[0], f) {
		t.Fatalf("snapshot mutated: %+v %v", page, err)
	}
	var key string
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM favorite_assets WHERE favorite_id=$1", f.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	r, _ := http.NewRequestWithContext(t.Context(), "DELETE", s.URL+"/api/v1/favorites/"+f.ID, nil)
	r.Header.Set("Cookie", "session="+bt)
	csrf, _ := testCSRFTokens.Load(bt)
	r.Header.Set("X-CSRF-Token", csrf.(string))
	resp, err := s.Client().Do(r)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatal(resp.StatusCode)
	}
	if _, err = services.Media.Files.Open(key); !os.IsNotExist(err) {
		t.Fatalf("private bytes not removed: %v", err)
	}
	favoriteDownload(t, s, bt, f.ID, a.ID, 404)
}

func TestFavoritesPaginationAndRejectedSources(t *testing.T) {
	services := fixture(t, false)
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	alice, at := account(t, services, "favorites-page")
	bob, bt := account(t, services, "favorites-page-other")
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "pages", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var saved []model.Favorite
	for i := 0; i < 5; i++ {
		m, _, e := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: fmt.Sprintf("favorites-page-%d", i), Content: fmt.Sprintf("row %d", i)})
		if e != nil {
			t.Fatal(e)
		}
		saved = append(saved, saveFavorite(t, s, at, cid, m.ID))
	}
	// Force equal timestamps to exercise the id tie breaker rather than sleeps.
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE favorites SET saved_at=$1", time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	cursor := ""
	last := ""
	for {
		p, e := services.Favorites.List(t.Context(), alice.ID, cursor, 2)
		if e != nil {
			t.Fatal(e)
		}
		for _, f := range p.Favorites {
			if seen[f.ID] || last != "" && last <= f.ID {
				t.Fatal("unstable pagination")
			}
			seen[f.ID] = true
			last = f.ID
		}
		if !p.HasMore {
			break
		}
		cursor = p.NextBeforeID
	}
	if len(seen) != 5 {
		t.Fatal(len(seen))
	}
	httpCall(t, s, bt, "GET", "/api/v1/favorites?before_id="+saved[0].ID, nil, 200)
	p, e := services.Favorites.List(t.Context(), bob.ID, "", 30)
	if e != nil || len(p.Favorites) != 0 {
		t.Fatal("private list leaked")
	}
	m, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "favorites-recalled", Content: "withdraw"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = services.Chat.Recall(t.Context(), alice.ID, cid, m.Seq); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, at, "POST", "/api/v1/favorites", map[string]string{"conversation_id": cid, "message_id": m.ID}, 400)
	// A system row is server generated; inserting it here avoids coupling to governance UI.
	id := model.NewID()
	if _, err = services.Store.DB.Exec(t.Context(), `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,kind) VALUES($1,$2,$3,$4,99,'system','system')`, id, cid, alice.ID, model.NewID()); err != nil {
		t.Fatal(err)
	}
	httpCall(t, s, at, "POST", "/api/v1/favorites", map[string]string{"conversation_id": cid, "message_id": id}, 400)
	svg := []byte("<svg onload='alert(1)'/>")
	a, _ := uploadMedia(t, s, at, cid, "unsafe.svg", "image/svg+xml", svg, 201)
	file, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "favorites-svg-file", MediaID: a.ID})
	if err != nil {
		t.Fatal(err)
	}
	f := saveFavorite(t, s, at, cid, file.ID)
	_, h := favoriteDownload(t, s, at, f.ID, a.ID, 200)
	if h.Get("Content-Disposition") == "inline" {
		t.Fatal("SVG inlined")
	}
	direct, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	dm, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: direct, ClientMsgID: "favorite-own-direct", Content: "own direct message"})
	if err != nil {
		t.Fatal(err)
	}
	df := saveFavorite(t, s, at, direct, dm.ID)
	if !df.IsOwn || df.ConversationName != bob.Name || df.SenderName != alice.Name {
		t.Fatalf("direct source name: %+v", df)
	}
}

func TestFavoritesConcurrentRecallAndFailedCopy(t *testing.T) {
	services := fixture(t, false)
	alice, at := account(t, services, "favorite-race")
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "race"})
	if err != nil {
		t.Fatal(err)
	}
	a, _ := uploadMedia(t, s, at, cid, "first.png", "image/png", []byte("first"), 201)
	b, _ := uploadMedia(t, s, at, cid, "missing.png", "image/png", []byte("missing"), 201)
	m, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "favorite-fail-copy", Blocks: []model.Block{{Type: "image", MediaID: a.ID}, {Type: "image", MediaID: b.ID}}})
	if err != nil {
		t.Fatal(err)
	}
	var missingKey string
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM message_media WHERE id=$1", b.ID).Scan(&missingKey); err != nil {
		t.Fatal(err)
	}
	if err = services.Media.Files.Remove(missingKey); err != nil {
		t.Fatal(err)
	}
	before, _ := filepath.Glob(filepath.Join(services.Media.Files.Dir(), "*", "*"))
	if _, err = services.Favorites.Create(t.Context(), alice.ID, cid, m.ID); err == nil {
		t.Fatal("missing source bytes accepted")
	}
	after, _ := filepath.Glob(filepath.Join(services.Media.Files.Dir(), "*", "*"))
	if len(before) != len(after) {
		t.Fatal("failed copy leaked bytes")
	}
	var count int
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM favorites").Scan(&count); err != nil || count != 0 {
		t.Fatal("partial favorite persisted")
	}
	// An in-flight recall holds the same conversation lock. A waiting capture
	// must re-read its source after the recall commits, never save stale content.
	tx, err := services.Store.DB.Begin(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(context.Background())
	if _, err = tx.Exec(t.Context(), "SELECT id FROM conversations WHERE id=$1 FOR UPDATE", cid); err != nil {
		t.Fatal(err)
	}
	result := make(chan error, 1)
	go func() { _, e := services.Favorites.Create(t.Context(), alice.ID, cid, m.ID); result <- e }()
	if _, err = tx.Exec(t.Context(), "UPDATE messages SET recalled_at=now() WHERE id=$1", m.ID); err != nil {
		t.Fatal(err)
	}
	if err = tx.Commit(t.Context()); err != nil {
		t.Fatal(err)
	}
	select {
	case err = <-result:
		status(t, err, 400)
	case <-time.After(5 * time.Second):
		t.Fatal("capture deadlocked with recall")
	}
	// Synthetic leftover task verifies durable retries remove unreferenced bytes.
	key := model.NewID()
	f, err := services.Media.Files.Create(key)
	if err != nil {
		t.Fatal(err)
	}
	f.Write([]byte("orphan"))
	f.Close()
	if _, err = services.Store.DB.Exec(t.Context(), "INSERT INTO favorite_asset_cleanup(storage_key) VALUES($1)", key); err != nil {
		t.Fatal(err)
	}
	if err = services.Favorites.Clean(t.Context()); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Media.Files.Open(key); !os.IsNotExist(err) {
		t.Fatal("orphan retry failed")
	}
}
