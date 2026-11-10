package api

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"image"
	"image/gif"
	"image/jpeg"
	"image/png"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

func libraryImage(t *testing.T, kind string) []byte {
	t.Helper()
	var out bytes.Buffer
	img := image.NewRGBA(image.Rect(0, 0, 1, 1))
	var err error
	switch kind {
	case "image/png":
		err = png.Encode(&out, img)
	case "image/jpeg":
		err = jpeg.Encode(&out, img, nil)
	case "image/gif":
		err = gif.Encode(&out, img, nil)
	case "image/webp":
		data, e := base64.StdEncoding.DecodeString("UklGRiIAAABXRUJQVlA4IBYAAAAwAQCdASoBAAEADsD+JaQAA3AAAAAA")
		if e != nil {
			t.Fatal(e)
		}
		return data
	}
	if err != nil {
		t.Fatal(err)
	}
	return out.Bytes()
}
func uploadLibrarySticker(t *testing.T, s *httptest.Server, token, kind string, data []byte, want int) model.Sticker {
	t.Helper()
	req, _ := http.NewRequestWithContext(t.Context(), "POST", s.URL+"/api/v1/stickers/upload?file_name=%E5%8E%9F%E5%9B%BE.png", bytes.NewReader(data))
	req.Header.Set("Content-Type", kind)
	if token != "" {
		req.Header.Set("Cookie", "session="+token)
		csrf, _ := testCSRFTokens.Load(token)
		req.Header.Set("X-CSRF-Token", csrf.(string))
	}
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != want {
		t.Fatalf("upload: %d want %d: %s", resp.StatusCode, want, raw)
	}
	if want != 201 {
		return model.Sticker{}
	}
	var envelope struct {
		Data model.Sticker `json:"data"`
	}
	if err = json.Unmarshal(raw, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Data
}

func TestStickerDirectUploadContract(t *testing.T) {
	services := fixture(t, false)
	alice, at := account(t, services, "upload-sticker-alice")
	_, bt := account(t, services, "upload-sticker-bob")
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	data := libraryImage(t, "image/png")
	uploadLibrarySticker(t, s, "", "image/png", data, 401)
	req, _ := http.NewRequestWithContext(t.Context(), "POST", s.URL+"/api/v1/stickers/upload", bytes.NewReader(data))
	req.Header.Set("Cookie", "session="+at)
	req.Header.Set("Content-Type", "image/png")
	resp, err := s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 403 {
		t.Fatal("missing csrf accepted")
	}
	for _, kind := range []string{"image/svg+xml", "text/plain", "video/mp4", "", "image/bmp"} {
		uploadLibrarySticker(t, s, at, kind, data, 415)
	}
	uploadLibrarySticker(t, s, at, "image/png", []byte("<svg xmlns=\"http://www.w3.org/2000/svg\"></svg>"), 415)
	uploadLibrarySticker(t, s, at, "image/png", []byte("<html>bad image</html>"), 415)
	uploadLibrarySticker(t, s, at, "image/png", libraryImage(t, "image/gif"), 415)
	uploadLibrarySticker(t, s, at, "image/png", nil, 400)
	uploadLibrarySticker(t, s, at, "image/png", make([]byte, logic.MaxImageBytes+1), 413)
	first := uploadLibrarySticker(t, s, at, "image/png", data, 201)
	if first.ID == "" || first.Kind != "image" || first.FileName != "原图.png" || first.ByteSize != int64(len(data)) {
		t.Fatalf("bad sticker: %+v", first)
	}
	dup := uploadLibrarySticker(t, s, at, "image/png; charset=binary", data, 201)
	if first != dup {
		t.Fatal("repeat changed sticker")
	}
	other := uploadLibrarySticker(t, s, bt, "image/png", data, 201)
	if other.ID == first.ID {
		t.Fatal("shared ownership")
	}
	for _, kind := range []string{"image/jpeg", "image/gif", "image/webp"} {
		uploadLibrarySticker(t, s, at, kind, libraryImage(t, kind), 201)
	}
	if !bytes.Equal(stickerDownload(t, s, at, first.ID, 200), data) {
		t.Fatal("bytes changed")
	}
	stickerDownload(t, s, bt, first.ID, 404)
	httpCall(t, s, bt, "DELETE", "/api/v1/stickers/"+first.ID, nil, 404)
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "target"})
	if err != nil {
		t.Fatal(err)
	}
	var prepared model.Media
	if err = json.Unmarshal(httpCall(t, s, at, "POST", "/api/v1/stickers/"+first.ID+"/prepare", map[string]string{"conversation_id": cid}, 201), &prepared); err != nil {
		t.Fatal(err)
	}
	var key string
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM stickers WHERE id=$1", first.ID).Scan(&key); err != nil {
		t.Fatal(err)
	}
	req, _ = http.NewRequestWithContext(t.Context(), "DELETE", s.URL+"/api/v1/stickers/"+first.ID, nil)
	req.Header.Set("Cookie", "session="+at)
	csrf, _ := testCSRFTokens.Load(at)
	req.Header.Set("X-CSRF-Token", csrf.(string))
	resp, err = s.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != 204 {
		t.Fatalf("delete: %d", resp.StatusCode)
	}
	stickerDownload(t, s, at, first.ID, 404)
	if f, e := services.Media.Files.Open(key); !os.IsNotExist(e) {
		if f != nil {
			f.Close()
		}
		t.Fatalf("bytes not removed: %v", e)
	}
	asset, err := services.Media.Retrieve(t.Context(), alice.ID, prepared.ID)
	if err != nil {
		t.Fatal(err)
	}
	f, err := services.Media.Open(asset)
	if err != nil {
		t.Fatal(err)
	}
	defer f.Close()
	actual, _ := io.ReadAll(f)
	if !bytes.Equal(actual, data) {
		t.Fatal("prepare lost bytes after sticker delete")
	}
}

func TestStickerDirectUploadConcurrencyLimitAndDurableFailure(t *testing.T) {
	services := fixture(t, false)
	alice, at := account(t, services, "upload-sticker-race")
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	config := services.Store.DB.Config()
	config.MaxConns = 2
	small, err := pgxpool.NewWithConfig(t.Context(), config)
	if err != nil {
		t.Fatal(err)
	}
	old := services.Store.DB
	services.Store.DB = small
	old.Close()
	data := libraryImage(t, "image/png")
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	var wg sync.WaitGroup
	results := make(chan model.Sticker, 8)
	failures := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			item, e := services.Stickers.Upload(ctx, alice.ID, "x.png", "image/png", int64(len(data)), bytes.NewReader(data))
			if e != nil {
				failures <- e
			} else {
				results <- item
			}
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
		if item != first {
			t.Fatal("concurrent dedup changed identity")
		}
	}
	if _, err = services.Store.DB.Exec(t.Context(), `INSERT INTO stickers(id,user_id,kind,source_key,text) SELECT 'upload_fill_'||n,$1,'emoji','upload_fill_'||n,'😀' FROM generate_series(1,199) n`, alice.ID); err != nil {
		t.Fatal(err)
	}
	dup := uploadLibrarySticker(t, s, at, "image/png", data, 201)
	if dup.ID != first.ID {
		t.Fatal("duplicate at cap changed identity")
	}
	uploadLibrarySticker(t, s, at, "image/png", append(append([]byte{}, data...), 0), 409)
	// Force a database insertion error after bytes are written; the failed asset
	// must remain registered for cleanup, including transaction rollback.
	if _, err = services.Store.DB.Exec(t.Context(), "DELETE FROM stickers WHERE id LIKE 'upload_fill_%'; ALTER TABLE stickers ADD CONSTRAINT reject_test_upload CHECK (file_name <> 'fail.png')"); err != nil {
		t.Fatal(err)
	}
	_, err = services.Stickers.Upload(t.Context(), alice.ID, "fail.png", "image/png", int64(len(data)+1), bytes.NewReader(append(append([]byte{}, data...), 1)))
	if err == nil {
		t.Fatal("insertion failure not triggered")
	}
	_, err = services.Stickers.Upload(t.Context(), alice.ID, "short.png", "image/png", int64(len(data)+1), bytes.NewReader(data))
	status(t, err, 400)
	var failedKey string
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM favorite_asset_cleanup WHERE storage_key NOT IN (SELECT storage_key FROM stickers WHERE storage_key IS NOT NULL) AND storage_key IN (SELECT storage_key FROM favorite_asset_cleanup) ORDER BY created_at DESC LIMIT 1").Scan(&failedKey); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Store.DB.Exec(t.Context(), "UPDATE favorite_asset_cleanup SET ready_at=now()"); err != nil {
		t.Fatal(err)
	}
	if err = services.Favorites.Clean(t.Context()); err != nil {
		t.Fatal(err)
	}
	if f, e := services.Media.Files.Open(failedKey); !os.IsNotExist(e) {
		if f != nil {
			f.Close()
		}
		t.Fatalf("orphan survives cleanup: %v", e)
	}
	var count int
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM favorite_asset_cleanup").Scan(&count); err != nil || count != 0 {
		t.Fatalf("cleanup queue: %d %v", count, err)
	}
}

func TestFavoriteMetadataPartialValidationPersistence(t *testing.T) {
	services := fixture(t, false)
	alice, at := account(t, services, "metadata-alice")
	_, bt := account(t, services, "metadata-bob")
	s := httptest.NewServer((&Server{Services: services}).Handler())
	defer s.Close()
	cid, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "snapshot group"})
	if err != nil {
		t.Fatal(err)
	}
	msg, _, err := services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: cid, ClientMsgID: "metadata-message", Content: "原文\n保持"})
	if err != nil {
		t.Fatal(err)
	}
	original := saveFavorite(t, s, at, cid, msg.ID)
	if original.Tags == nil || len(original.Tags) != 0 || original.Note != "" {
		t.Fatal("metadata defaults invalid")
	}
	path := "/api/v1/favorites/" + original.ID
	httpCall(t, s, "", "PATCH", path, map[string]any{"note": "x"}, 401)
	httpCall(t, s, bt, "PATCH", path, map[string]any{"note": "x"}, 404)
	httpCall(t, s, at, "PATCH", "/api/v1/favorites/unknown", map[string]any{"note": "x"}, 404)
	patch := func(body any) model.Favorite {
		t.Helper()
		var f model.Favorite
		if err := json.Unmarshal(httpCall(t, s, at, "PATCH", path, body, 200), &f); err != nil {
			t.Fatal(err)
		}
		return f
	}
	first := patch(map[string]any{"tags": []string{" 项目 ", "项目", "　", "😀"}, "note": "个人备注\n第二行"})
	if !reflect.DeepEqual(first.Tags, []string{"项目", "😀"}) || first.Note != "个人备注\n第二行" {
		t.Fatalf("normalization failed: %+v", first)
	}
	second := patch(map[string]any{"note": "只修改备注\n保留"})
	if !reflect.DeepEqual(first.Tags, second.Tags) {
		t.Fatal("note erased tags")
	}
	third := patch(map[string]any{"tags": []string{strings.Repeat("😀", 20)}})
	if third.Note != second.Note {
		t.Fatal("tags erased note")
	}
	same := patch(map[string]any{})
	if !reflect.DeepEqual(same, third) {
		t.Fatal("empty patch changed metadata")
	}
	for _, body := range []any{map[string]any{"tags": []string{strings.Repeat("中", 21)}}, map[string]any{"note": strings.Repeat("😀", 2001)}, map[string]any{"tags": nil}, map[string]any{"note": nil}, map[string]any{"tags": []any{nil}}, map[string]any{"tags": "x"}, map[string]any{"note": true}, map[string]any{"message": "overwrite"}} {
		httpCall(t, s, at, "PATCH", path, body, 400)
	}
	many := []string{}
	for i := 0; i < 11; i++ {
		many = append(many, fmt.Sprint(i))
	}
	httpCall(t, s, at, "PATCH", path, map[string]any{"tags": many}, 400)
	final := patch(map[string]any{"note": strings.Repeat("😀", 2000)})
	dup := saveFavorite(t, s, at, cid, msg.ID)
	if !reflect.DeepEqual(dup, final) {
		t.Fatal("repeat favorite lost metadata")
	}
	var page model.FavoritePage
	if err = json.Unmarshal(httpCall(t, s, at, "GET", "/api/v1/favorites", nil, 200), &page); err != nil || len(page.Favorites) != 1 || !reflect.DeepEqual(page.Favorites[0], final) {
		t.Fatalf("list persistence mismatch: %+v %v", page, err)
	}
	final.Tags = original.Tags
	final.Note = original.Note
	if !reflect.DeepEqual(final, original) {
		t.Fatal("metadata changed immutable snapshot")
	}
	cleared := patch(map[string]any{"tags": []string{}, "note": ""})
	if cleared.Tags == nil || len(cleared.Tags) != 0 || cleared.Note != "" {
		t.Fatal("explicit clear failed")
	}
}
