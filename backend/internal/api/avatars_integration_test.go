package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

// pngBytes is a tiny valid PNG header; the endpoint validates the declared
// content type and size, not the decoded pixels.
var pngBytes = []byte{0x89, 'P', 'N', 'G', '\r', '\n', 0x1a, '\n', 0, 0, 0, 13}

func TestAvatarUploadAndVisibility(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	if _, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}}); err != nil {
		t.Fatal(err)
	}

	// Upload sets the caller's avatar and echoes the fresh profile.
	request, err := http.NewRequest("POST", server.URL+"/api/v1/me/avatar?file_name=me.png", bytes.NewReader(pngBytes))
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Content-Type", "image/png")
	request.Header.Set("Cookie", "session="+aliceToken)
	if csrf, ok := testCSRFTokens.Load(aliceToken); ok {
		request.Header.Set("X-CSRF-Token", csrf.(string))
	}
	response, err := server.Client().Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != 200 {
		t.Fatalf("avatar upload: %s", response.Status)
	}
	var account model.User
	if err := json.NewDecoder(response.Body).Decode(&struct{ Data *model.User }{&account}); err != nil {
		t.Fatal(err)
	}
	response.Body.Close()
	if account.Avatar == "" {
		t.Fatal("avatar id missing after upload")
	}

	// The avatar media is downloadable by any authenticated user, not just the
	// owner: avatars exist to be seen.
	mediaRequest, err := http.NewRequest("GET", server.URL+"/api/v1/media/"+account.Avatar, nil)
	if err != nil {
		t.Fatal(err)
	}
	mediaRequest.Header.Set("Cookie", "session="+bobToken)
	mediaResponse, err := server.Client().Do(mediaRequest)
	if err != nil {
		t.Fatal(err)
	}
	if mediaResponse.StatusCode != 200 {
		body, _ := io.ReadAll(mediaResponse.Body)
		t.Fatalf("avatar download as other user: %s body=%s", mediaResponse.Status, body)
	}
	mediaResponse.Body.Close()

	// DEBUG: direct retrieve
	if _, retrieveErr := services.Media.Retrieve(ctx, bob.ID, account.Avatar); retrieveErr != nil {
		t.Logf("DEBUG retrieve error: %v", retrieveErr)
	}

	// The friend and member payloads carry the avatar id for rendering.
	list := httpCall(t, server, bobToken, "GET", "/api/v1/users", nil, 200)
	var users []model.User
	if err := json.Unmarshal(list, &users); err != nil {
		t.Fatal(err)
	}
	if len(users) != 1 || users[0].ID != alice.ID || users[0].Avatar == "" {
		t.Fatalf("avatar missing from the users list: %+v", users)
	}
	members := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var conversations []model.Conversation
	if err := json.Unmarshal(members, &conversations); err != nil {
		t.Fatal(err)
	}
	if len(conversations) != 1 {
		t.Fatalf("unexpected conversations: %+v", conversations)
	}
	var aliceMember *model.Member
	for index := range conversations[0].Members {
		if conversations[0].Members[index].ID == alice.ID {
			aliceMember = &conversations[0].Members[index]
		}
	}
	if aliceMember == nil || aliceMember.Avatar == "" {
		t.Fatalf("avatar missing from conversation members: %+v", conversations[0].Members)
	}

	// Clearing the avatar empties the field without deleting the media row.
	cleared := httpCall(t, server, aliceToken, "DELETE", "/api/v1/me/avatar", nil, 200)
	var reset model.User
	if err := json.Unmarshal(cleared, &reset); err != nil {
		t.Fatal(err)
	}
	if reset.Avatar != "" {
		t.Fatalf("avatar not cleared: %+v", reset)
	}
}

func TestAvatarValidation(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	_, aliceToken := account(t, services, "alice")

	tooBig := bytes.Repeat([]byte{0x89, 'P', 'N', 'G'}, (2<<20)/3+10)
	upload := func(body []byte, contentType string) int {
		request, err := http.NewRequest("POST", server.URL+"/api/v1/me/avatar", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		request.Header.Set("Content-Type", contentType)
		request.Header.Set("Cookie", "session="+aliceToken)
		if csrf, ok := testCSRFTokens.Load(aliceToken); ok {
			request.Header.Set("X-CSRF-Token", csrf.(string))
		}
		response, err := server.Client().Do(request)
		if err != nil {
			t.Fatal(err)
		}
		defer response.Body.Close()
		return response.StatusCode
	}
	if code := upload(tooBig, "image/png"); code != 413 {
		t.Fatalf("oversized avatar: wanted 413 got %d", code)
	}
	if code := upload(pngBytes, "video/mp4"); code != 415 {
		t.Fatalf("non-image avatar: wanted 415 got %d", code)
	}
}

func TestPokeCreatesSystemNotice(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, carolToken := account(t, services, "carol")
	direct, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+direct+"/poke", model.PokeInput{UserID: bob.ID}, 200)
	// A non-member cannot be poked and a non-member cannot poke.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+direct+"/poke", model.PokeInput{UserID: carol.ID}, 404)
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+direct+"/poke", model.PokeInput{UserID: alice.ID}, 404)

	history := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+direct+"/messages", nil, 200)
	var messages []model.Message
	if err := json.Unmarshal(history, &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || messages[0].Kind != "system" || !strings.Contains(messages[0].Content, "拍了拍") {
		t.Fatalf("poke notice missing: %+v", messages)
	}
}
