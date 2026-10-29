package api

import (
	"bytes"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

// groupFilesPage uses the existing conversation search endpoint with an explicit
// empty query, as the group file browser does.
func groupFilesPage(t *testing.T, server *httptest.Server, token, group, query string) model.SearchPage {
	t.Helper()
	raw := httpCall(t, server, token, "GET", "/api/v1/conversations/"+group+"/messages/search?type=files&q="+query, nil, 200)
	var page model.SearchPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	return page
}

func assertGroupFiles(t *testing.T, page model.SearchPage, messages ...model.Message) {
	t.Helper()
	if len(page.Messages) != len(messages) {
		t.Fatalf("wanted %d files, got %+v", len(messages), page)
	}
	for i, want := range messages {
		got := page.Messages[i]
		if got.ID != want.ID || got.Seq != want.Seq || got.Kind != "file" || got.MediaID != want.MediaID || got.Media == nil || want.Media == nil || *got.Media != *want.Media {
			t.Fatalf("file %d: wanted %+v with media %+v, got %+v with media %+v", i, want, want.Media, got, got.Media)
		}
	}
}

func TestGroupFilesEmptyQueryListsSentFiles(t *testing.T) {
	services, server, alice, aliceToken, _, bobToken, _, _, group := groupFixture(t)
	firstUpload, _ := uploadMedia(t, server, aliceToken, group, "report.txt", "text/plain", []byte("first report"), 201)
	first := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "group-file-first", "content": "quarterly report", "media_id": firstUpload.ID}, 201)
	secondUpload, _ := uploadMedia(t, server, bobToken, group, "archive.zip", "application/zip", []byte("PK archive"), 201)
	second := sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "group-file-second", "content": "", "media_id": secondUpload.ID}, 201)

	// Uploaded drafts, text, images, videos and files in another group are not
	// sent ordinary files belonging to this group.
	uploadMedia(t, server, aliceToken, group, "draft.txt", "text/plain", []byte("unsent"), 201)
	sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "group-file-text", "content": "report.txt"}, 201)
	for _, item := range []struct{ name, contentType string }{{"photo.png", "image/png"}, {"movie.mp4", "video/mp4"}} {
		uploaded, _ := uploadMedia(t, server, aliceToken, group, item.name, item.contentType, []byte("media"), 201)
		sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "group-file-" + item.name, "content": "", "media_id": uploaded.ID}, 201)
	}
	other, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "other files"})
	if err != nil {
		t.Fatal(err)
	}
	foreign, _ := uploadMedia(t, server, aliceToken, other, "foreign.txt", "text/plain", []byte("other group"), 201)
	sendMessage(t, server, aliceToken, other, map[string]any{"client_msg_id": "group-file-foreign", "content": "", "media_id": foreign.ID}, 201)

	page := groupFilesPage(t, server, bobToken, group, "")
	assertGroupFiles(t, page, second, first)
	if page.HasMore || page.NextBeforeSeq != first.Seq {
		t.Fatalf("unexpected complete page: %+v", page)
	}
	// Paging must enumerate both files while skipping non-file messages.
	page = groupFilesPage(t, server, bobToken, group, "&limit=1")
	assertGroupFiles(t, page, second)
	if !page.HasMore || page.NextBeforeSeq != second.Seq {
		t.Fatalf("unexpected first page: %+v", page)
	}
	page = groupFilesPage(t, server, bobToken, group, "&limit=1&before_seq="+itoa(page.NextBeforeSeq))
	assertGroupFiles(t, page, first)
	if page.HasMore {
		t.Fatalf("unexpected extra file page: %+v", page)
	}
}

func TestGroupFilesMembershipControlsHistoryAndDownload(t *testing.T) {
	services, server, _, aliceToken, _, bobToken, carol, carolToken, group := groupFixture(t)
	newcomer, newcomerToken := account(t, services, "newcomer")
	_, outsiderToken := account(t, services, "outsider")
	body := []byte("file sent before the newcomer joined")
	uploaded, _ := uploadMedia(t, server, aliceToken, group, "history.txt", "text/plain", body, 201)
	old := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "group-file-history", "content": "", "media_id": uploaded.ID}, 201)
	path := "/api/v1/conversations/" + group

	// Before joining, the newcomer has the same restrictions as an outsider.
	for _, token := range []string{newcomerToken, outsiderToken} {
		httpCall(t, server, token, "GET", path+"/messages/search?type=files&q=", nil, 404)
		download(t, server, token, uploaded.ID, 404)
	}
	httpCall(t, server, aliceToken, "POST", path+"/members", model.AddMembersInput{MemberIDs: []string{newcomer.ID}}, 200)
	for _, token := range []string{newcomerToken, bobToken, carolToken} {
		assertGroupFiles(t, groupFilesPage(t, server, token, group, ""), old)
		content, contentType := download(t, server, token, uploaded.ID, 200)
		if !bytes.Equal(content, body) || contentType != "text/plain" {
			t.Fatalf("historical download mismatch: %q %q", content, contentType)
		}
	}

	// Both voluntary departure and removal revoke access to previously readable
	// files, including a saved direct download URL.
	httpCall(t, server, newcomerToken, "POST", path+"/members/leave", nil, 200)
	httpCall(t, server, aliceToken, "DELETE", path+"/members", map[string]string{"user_id": carol.ID}, 200)
	for _, token := range []string{newcomerToken, carolToken, outsiderToken} {
		httpCall(t, server, token, "GET", path+"/messages/search?type=files&q=", nil, 404)
		download(t, server, token, uploaded.ID, 404)
	}
	assertGroupFiles(t, groupFilesPage(t, server, bobToken, group, ""), old)
	download(t, server, bobToken, uploaded.ID, 200)
}

func TestGroupFilesRecallHidesListingAndDownload(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, _, group := groupFixture(t)
	uploaded, _ := uploadMedia(t, server, aliceToken, group, "withdraw.txt", "text/plain", []byte("withdrawn bytes"), 201)
	withdrawn := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "group-file-withdraw", "content": "", "media_id": uploaded.ID}, 201)
	keptUpload, _ := uploadMedia(t, server, aliceToken, group, "keep.txt", "text/plain", []byte("kept bytes"), 201)
	kept := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "group-file-keep", "content": "", "media_id": keptUpload.ID}, 201)
	assertGroupFiles(t, groupFilesPage(t, server, bobToken, group, ""), kept, withdrawn)
	download(t, server, bobToken, uploaded.ID, 200)

	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(withdrawn.Seq)+"/recall", nil, 200)
	for _, token := range []string{aliceToken, bobToken} {
		page := groupFilesPage(t, server, token, group, "")
		assertGroupFiles(t, page, kept)
		if page.HasMore {
			t.Fatalf("recalled file counted as another page: %+v", page)
		}
		download(t, server, token, uploaded.ID, 404)
		content, _ := download(t, server, token, keptUpload.ID, 200)
		if !bytes.Equal(content, []byte("kept bytes")) {
			t.Fatalf("unrecalled file changed: %q", content)
		}
	}
}
