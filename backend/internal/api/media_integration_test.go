package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

// mediaFixture reuses the shared integration fixture, which already stores
// media in a per-test temporary directory.
func mediaFixture(t *testing.T) *svc.ServiceContext {
	t.Helper()
	return fixture(t, false)
}

func uploadMedia(t *testing.T, server *httptest.Server, token, conversationID, fileName, contentType string, body []byte, expected int) (model.MediaUpload, []byte) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), "POST", server.URL+"/api/v1/conversations/"+conversationID+"/media?file_name="+fileName, bytes.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+token)
	request.Header.Set("Content-Type", contentType)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected {
		t.Fatalf("upload: wanted %d got %d: %s", expected, response.StatusCode, content)
	}
	var envelope struct {
		Data model.MediaUpload `json:"data"`
	}
	_ = json.Unmarshal(content, &envelope)
	return envelope.Data, content
}

func download(t *testing.T, server *httptest.Server, token, mediaID string, expected int) ([]byte, string) {
	t.Helper()
	request, err := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/api/v1/media/"+mediaID, nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Cookie", "session="+token)
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	content, _ := io.ReadAll(response.Body)
	if response.StatusCode != expected {
		t.Fatalf("download: wanted %d got %d: %s", expected, response.StatusCode, content)
	}
	return content, response.Header.Get("Content-Type")
}

func TestMediaUploadAndSend(t *testing.T) {
	services := mediaFixture(t)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	_, charlieToken := account(t, services, "charlie")
	conversation, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "media", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	image := []byte("\x89PNG\r\n\x1a\n fake image bytes")
	uploaded, _ := uploadMedia(t, server, aliceToken, conversation, "photo.png", "image/png", image, 201)
	if uploaded.ID == "" || uploaded.MediaType != "image" || uploaded.ByteSize != int64(len(image)) || uploaded.ContentType != "image/png" {
		t.Fatalf("unexpected upload metadata: %+v", uploaded)
	}

	// The stored bytes are readable by a member and never by an outsider.
	content, contentType := download(t, server, bobToken, uploaded.ID, 200)
	if !bytes.Equal(content, image) || contentType != "image/png" {
		t.Fatalf("download mismatch: %q %q", content, contentType)
	}
	download(t, server, charlieToken, uploaded.ID, 404)

	// A caption is optional, and the kind is taken from the upload.
	sent := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": "media-send-1", "content": "", "media_id": uploaded.ID}, 201)
	var message model.Message
	if err := json.Unmarshal(sent, &message); err != nil {
		t.Fatal(err)
	}
	if message.Kind != "image" || message.MediaID != uploaded.ID || message.Media == nil || message.Media.FileName != "photo.png" {
		t.Fatalf("unexpected media message: %+v", message)
	}

	// Syncing reproduces the same attachment metadata for other members.
	page := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+conversation+"/sync?after_seq=0", nil, 200)
	var synced model.SyncPage
	if err := json.Unmarshal(page, &synced); err != nil {
		t.Fatal(err)
	}
	if len(synced.Messages) != 1 || synced.Messages[0].Kind != "image" || synced.Messages[0].Media == nil || synced.Messages[0].Media.ID != uploaded.ID {
		t.Fatalf("unexpected synced page: %+v", synced.Messages)
	}
}

func TestMediaRejectsInvalidUploads(t *testing.T) {
	services := mediaFixture(t)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	_, outsiderToken := account(t, services, "outsider")
	conversation, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "media"})
	if err != nil {
		t.Fatal(err)
	}

	uploadMedia(t, server, aliceToken, conversation, "notes.txt", "text/plain", []byte("plain"), 415)
	uploadMedia(t, server, aliceToken, conversation, "empty.png", "image/png", nil, 400)
	uploadMedia(t, server, outsiderToken, conversation, "photo.png", "image/png", []byte("no access"), 404)

	// An image larger than the per-type limit is refused before storage.
	oversized := bytes.Repeat([]byte("x"), int(logic.MaxImageBytes)+1)
	uploadMedia(t, server, aliceToken, conversation, "huge.png", "image/png", oversized, 413)

	// Media cannot be attached to another user's conversation or by a non-member.
	uploaded, _ := uploadMedia(t, server, aliceToken, conversation, "photo.png", "image/png", []byte("image"), 201)
	httpCall(t, server, outsiderToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": "outsider-send", "content": "", "media_id": uploaded.ID}, 404)
	httpCall(t, server, outsiderToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": "outsider-send-2", "content": "hello"}, 404)

	// An empty message without media stays invalid, as before.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": "empty-send", "content": ""}, 400)
	// A media id that does not exist is refused.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": "missing-media", "content": "", "media_id": "does-not-exist"}, 404)
	// A media id from a different conversation is refused.
	other, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+other+"/messages", map[string]string{"client_msg_id": "cross-conversation", "content": "", "media_id": uploaded.ID}, 404)
}

func TestMediaRejectsForeignOwner(t *testing.T) {
	services := mediaFixture(t)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	conversation, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "shared", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, _ := uploadMedia(t, server, aliceToken, conversation, "photo.png", "image/png", []byte("image"), 201)
	// Bob is a member but did not upload the file, so he may not send it as his own.
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": "borrowed-media", "content": "", "media_id": uploaded.ID}, 404)
	// Bob can still read it, because reading only requires membership.
	download(t, server, bobToken, uploaded.ID, 200)
}

// A missing on-disk file must surface as not found instead of a server error.
func TestMediaMissingFileIsNotFound(t *testing.T) {
	services := mediaFixture(t)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	conversation, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "missing"})
	if err != nil {
		t.Fatal(err)
	}
	uploaded, _ := uploadMedia(t, server, aliceToken, conversation, "photo.png", "image/png", []byte("image"), 201)
	var storageKey string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT storage_key FROM message_media WHERE id=$1", uploaded.ID).Scan(&storageKey); err != nil {
		t.Fatal(err)
	}
	if err := services.Media.Files.Remove(storageKey); err != nil {
		t.Fatal(err)
	}
	download(t, server, aliceToken, uploaded.ID, 404)
}
