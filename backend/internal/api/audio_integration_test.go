package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

func TestAudioUploadSendSyncDownload(t *testing.T) {
	services := mediaFixture(t)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "audio-alice")
	bob, bobToken := account(t, services, "audio-bob")
	_, outsiderToken := account(t, services, "audio-outsider")
	conversation, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "audio", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	for _, item := range []struct{ contentType, extension string }{
		{"audio/webm", ".webm"}, {"audio/ogg", ".ogg"}, {"audio/mp4", ".m4a"}, {"audio/mpeg", ".mp3"}, {"audio/wav", ".wav"},
		{"audio/webm;codecs=opus", ".webm"},
	} {
		t.Run(item.contentType, func(t *testing.T) {
			body := []byte("recording bytes: " + item.contentType)
			// No file_name exercises the route's MIME-to-extension fallback.
			upload, _ := uploadMedia(t, server, aliceToken, conversation, "", item.contentType, body, 201)
			baseType := item.contentType
			if baseType == "audio/webm;codecs=opus" {
				baseType = "audio/webm"
			}
			if upload.MediaType != "audio" || upload.ContentType != baseType || upload.FileName != "upload"+item.extension || upload.ByteSize != int64(len(body)) {
				t.Fatalf("unexpected audio metadata: %+v", upload)
			}
			sent := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": model.NewID(), "content": "", "media_id": upload.ID}, 201)
			var message model.Message
			if err := json.Unmarshal(sent, &message); err != nil {
				t.Fatal(err)
			}
			if message.Kind != "audio" || message.Media == nil || message.Media.MediaType != "audio" || message.MediaID != upload.ID {
				t.Fatalf("unexpected audio message: %+v", message)
			}
			content, contentType := download(t, server, bobToken, upload.ID, 200)
			if !bytes.Equal(content, body) || contentType != baseType {
				t.Fatalf("audio download mismatch: %q %q", content, contentType)
			}
			download(t, server, outsiderToken, upload.ID, 404)
			httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+conversation+"/messages", map[string]string{"client_msg_id": model.NewID(), "media_id": upload.ID}, 404)
			request, err := http.NewRequestWithContext(t.Context(), "GET", server.URL+"/api/v1/media/"+upload.ID, nil)
			if err != nil {
				t.Fatal(err)
			}
			request.Header.Set("Cookie", "session="+bobToken)
			request.Header.Set("Range", "bytes=0-3")
			response, err := server.Client().Do(request)
			if err != nil {
				t.Fatal(err)
			}
			defer response.Body.Close()
			partial, err := io.ReadAll(response.Body)
			if err != nil || response.StatusCode != 206 || !bytes.Equal(partial, body[:4]) || response.Header.Get("Content-Disposition") != "inline" || response.Header.Get("X-Content-Type-Options") != "nosniff" {
				t.Fatalf("audio range playback: status=%d body=%q headers=%v err=%v", response.StatusCode, partial, response.Header, err)
			}
		})
	}
	page := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+conversation+"/sync?after_seq=0", nil, 200)
	var synced model.SyncPage
	if err := json.Unmarshal(page, &synced); err != nil {
		t.Fatal(err)
	}
	if len(synced.Messages) != 6 {
		t.Fatalf("expected six audio messages: %+v", synced.Messages)
	}
	for _, message := range synced.Messages {
		if message.Kind != "audio" || message.Media == nil || message.Media.MediaType != "audio" {
			t.Fatalf("sync lost audio metadata: %+v", message)
		}
	}

	// Rolling back preserves attachments and restores the pre-audio constraints.
	down, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", "000026_audio_messages.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.Store.DB.Exec(t.Context(), string(down)); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM messages m JOIN message_media mm ON mm.id=m.media_id WHERE m.kind='file' AND mm.media_type='file'").Scan(&count); err != nil || count != 6 {
		t.Fatalf("rollback lost audio messages: count=%d err=%v", count, err)
	}
	if _, err := services.Store.DB.Exec(t.Context(), "UPDATE message_media SET media_type='audio'"); err == nil {
		t.Fatal("rollback still allows audio media")
	}
	if _, err := services.Store.DB.Exec(t.Context(), "UPDATE messages SET kind='audio'"); err == nil {
		t.Fatal("rollback still allows audio messages")
	}
	up, err := os.ReadFile(filepath.Join("..", "..", "..", "database", "migrations", "000026_audio_messages.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := services.Store.DB.Exec(t.Context(), string(up)); err != nil {
		t.Fatal(err)
	}
}

func TestAudioUploadSizeBoundary(t *testing.T) {
	services := mediaFixture(t)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, token := account(t, services, "audio-size")
	conversation, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "audio sizes"})
	if err != nil {
		t.Fatal(err)
	}
	body := bytes.Repeat([]byte("x"), logic.MaxAudioBytes+1)
	upload, _ := uploadMedia(t, server, token, conversation, "limit.webm", "audio/webm", body[:logic.MaxAudioBytes], 201)
	if upload.ByteSize != logic.MaxAudioBytes || upload.MediaType != "audio" {
		t.Fatalf("exact 10 MiB limit should be accepted: %+v", upload)
	}
	for _, contentType := range []string{"audio/webm", "audio/ogg", "audio/mp4", "audio/mpeg", "audio/wav"} {
		uploadMedia(t, server, token, conversation, "large.audio", contentType, body, 413)
	}
	uploadMedia(t, server, token, conversation, "empty.webm", "audio/webm", nil, 400)
	var count int
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM message_media").Scan(&count); err != nil || count != 1 {
		t.Fatalf("rejected uploads created rows: count=%d err=%v", count, err)
	}
}
