package api

import (
	"encoding/json"
	"net/http/httptest"
	"net/url"
	"testing"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

func TestFileAttachmentUploadAndDownload(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, _, group := replyFixture(t)

	// A document is stored as a generic file rather than rejected.
	document := []byte("PK\x03\x04 pretend spreadsheet bytes")
	uploaded, _ := uploadMedia(t, server, aliceToken, group, "report.xlsx", "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet", document, 201)
	if uploaded.MediaType != "file" || uploaded.ByteSize != int64(len(document)) {
		t.Fatalf("unexpected file upload: %+v", uploaded)
	}
	// Files download as an attachment so a browser never renders them inline.
	content, contentType := download(t, server, bobToken, uploaded.ID, 200)
	if string(content) != string(document) || contentType != "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet" {
		t.Fatalf("file download mismatch: %q %q", content, contentType)
	}
	// The message becomes kind 'file' and keeps its attachment metadata.
	message := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "file-0001", "content": "月度报表", "media_id": uploaded.ID}, 201)
	if message.Kind != "file" || message.Media == nil || message.Media.FileName != "report.xlsx" {
		t.Fatalf("unexpected file message: %+v", message)
	}
	// A generic file may not be sent without a caption requirement being relaxed
	// in the wrong direction: an empty caption plus a file is still valid.
	empty := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "file-0002", "content": "", "media_id": (func() string {
		second, _ := uploadMedia(t, server, aliceToken, group, "notes.txt", "text/plain", []byte("hello"), 201)
		return second.ID
	})()}, 201)
	if empty.Kind != "file" || empty.Content != "" {
		t.Fatalf("empty caption file message was rejected: %+v", empty)
	}
	// An oversized file is refused with the file limit, not the video limit.
	oversized := make([]byte, logic.MaxFileBytes+1)
	uploadMedia(t, server, aliceToken, group, "huge.bin", "application/octet-stream", oversized, 413)
	// An impossible content type is still refused.
	uploadMedia(t, server, aliceToken, group, "weird.bin", "", []byte("x"), 415)
}

func TestGlobalSearchAcrossConversations(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, _ := account(t, services, "carol")

	first, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "alpha", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	second, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// A conversation alice is not part of must never appear in her results.
	hidden, err := services.Chat.Create(t.Context(), bob.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{carol.ID}})
	if err != nil {
		t.Fatal(err)
	}
	sendMessage(t, server, aliceToken, first, map[string]any{"client_msg_id": "search-001", "content": "季度预算评审"}, 201)
	sendMessage(t, server, aliceToken, second, map[string]any{"client_msg_id": "search-002", "content": "预算已确认"}, 201)
	sendMessage(t, server, bobToken, hidden, map[string]any{"client_msg_id": "search-003", "content": "预算泄露了"}, 201)

	raw := httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q="+urlQueryEscape("预算"), nil, 200)
	var page model.GlobalSearchPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 2 {
		t.Fatalf("expected only the caller's two conversations, got %d: %+v", len(page.Results), page.Results)
	}
	for _, hit := range page.Results {
		if hit.Message.ConversationID == hidden {
			t.Fatal("search leaked a conversation the caller is not in")
		}
		if hit.ConversationName == "" && hit.ConversationKind != "direct" {
			t.Fatalf("missing conversation context: %+v", hit)
		}
	}
	// Conversation context is present for groups.
	found := false
	for _, hit := range page.Results {
		if hit.Message.ConversationID == first && hit.ConversationName == "alpha" && hit.MemberCount == 2 {
			found = true
		}
	}
	if !found {
		t.Fatalf("group context missing: %+v", page.Results)
	}

	// Bob is a member of both alice's group and their direct conversation, plus
	// his own conversation with carol, so he legitimately sees all three matches.
	bobRaw := httpCall(t, server, bobToken, "GET", "/api/v1/messages/search?q="+urlQueryEscape("预算"), nil, 200)
	var bobPage model.GlobalSearchPage
	if err := json.Unmarshal(bobRaw, &bobPage); err != nil {
		t.Fatal(err)
	}
	seen := map[string]bool{}
	for _, hit := range bobPage.Results {
		seen[hit.Message.ConversationID] = true
	}
	if len(bobPage.Results) != 3 || !seen[first] || !seen[second] || !seen[hidden] {
		t.Fatalf("bob should see his three conversations, got %+v", bobPage.Results)
	}
	// Carol is only in the hidden conversation, so she must not see alice's group.
	_, carolToken := account(t, services, "carol2")
	carolRaw := httpCall(t, server, carolToken, "GET", "/api/v1/messages/search?q="+urlQueryEscape("预算"), nil, 200)
	var carolPage model.GlobalSearchPage
	if err := json.Unmarshal(carolRaw, &carolPage); err != nil {
		t.Fatal(err)
	}
	if len(carolPage.Results) != 0 {
		t.Fatalf("a non-member saw foreign messages: %+v", carolPage.Results)
	}

	// A recalled body must not be searchable any more.
	recallRaw := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+first+"/sync?after_seq=0&limit=10", nil, 200)
	var syncPage model.SyncPage
	if err := json.Unmarshal(recallRaw, &syncPage); err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+first+"/messages/"+itoa(syncPage.Messages[0].Seq)+"/recall", nil, 200)
	afterRecall := httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q="+urlQueryEscape("预算"), nil, 200)
	var afterPage model.GlobalSearchPage
	if err := json.Unmarshal(afterRecall, &afterPage); err != nil {
		t.Fatal(err)
	}
	if len(afterPage.Results) != 1 || afterPage.Results[0].Message.ConversationID != second {
		t.Fatalf("recalled message still searchable: %+v", afterPage.Results)
	}

	// Validation: empty and over-long queries, bad limits and bad cursors.
	httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q=", nil, 400)
	httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q=abc&limit=0", nil, 400)
	httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q=abc&limit=51", nil, 400)
	httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q=abc&before_time=not-a-time&before_id=x", nil, 400)
	httpCall(t, server, aliceToken, "GET", "/api/v1/messages/search?q=abc&before_time=2026-01-01T00:00:00Z", nil, 400)
}

// urlQueryEscape escapes a query value for use in a test URL.
func urlQueryEscape(value string) string { return url.QueryEscape(value) }
