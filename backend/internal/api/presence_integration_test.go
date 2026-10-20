package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"
	"time"

	"websocket_chat_go/backend/internal/model"
)

// TestPresenceReportsConnectedUsers proves the presence markers are actually
// read, not merely written: the endpoint must report a user as online only while
// a real WebSocket connection is held, and as offline once it closes.
func TestPresenceReportsConnectedUsers(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "presence-alice")
	bob, bobToken := account(t, services, "presence-bob")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "presence", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	_ = group

	// Bob's connection is registered by dialing; it also writes the marker.
	receiver := dial(t, server, bobToken, "presence-device")

	raw := httpCall(t, server, aliceToken, "POST", "/api/v1/presence", map[string]any{"user_ids": []string{alice.ID, bob.ID}}, 200)
	var result struct {
		Online map[string]bool `json:"online"`
	}
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Online[bob.ID] {
		t.Fatalf("bob holds a live connection but is reported offline: %+v", result.Online)
	}
	// Alice holds no connection in this test, so she must be reported offline.
	if result.Online[alice.ID] {
		t.Fatalf("a user without a connection must not be online: %+v", result.Online)
	}

	// The conversation payload carries the same flag per member.
	list := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var conversations []model.Conversation
	if err := json.Unmarshal(list, &conversations); err != nil || len(conversations) != 1 {
		t.Fatalf("unexpected conversations: %+v %v", conversations, err)
	}
	onlineByID := map[string]bool{}
	for _, member := range conversations[0].Members {
		onlineByID[member.ID] = member.Online
	}
	if !onlineByID[bob.ID] || onlineByID[alice.ID] {
		t.Fatalf("member online flags are wrong: %+v", onlineByID)
	}

	// Closing the connection must make the user offline. The marker is deleted by
	// the write loop's cleanup, so this also proves the delete path is exercised.
	_ = receiver.connection.CloseNow()
	deadline := time.Now().Add(10 * time.Second)
	for {
		after := httpCall(t, server, aliceToken, "POST", "/api/v1/presence", map[string]any{"user_ids": []string{bob.ID}}, 200)
		var second struct {
			Online map[string]bool `json:"online"`
		}
		if err := json.Unmarshal(after, &second); err != nil {
			t.Fatal(err)
		}
		if !second.Online[bob.ID] {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("presence stayed online after the connection closed")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

// A malformed or oversized request must be rejected rather than answered with a
// silently wrong map.
func TestPresenceValidatesInput(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	_, aliceToken := account(t, services, "presence-validate")

	huge := make([]string, 1001)
	for index := range huge {
		huge[index] = "user"
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/presence", map[string]any{"user_ids": huge}, 400)
	// An empty list is a valid, empty answer rather than an error.
	raw := httpCall(t, server, aliceToken, "POST", "/api/v1/presence", map[string]any{"user_ids": []string{}}, 200)
	var result struct {
		Online map[string]bool `json:"online"`
	}
	if err := json.Unmarshal(raw, &result); err != nil || len(result.Online) != 0 {
		t.Fatalf("an empty request should answer with an empty map: %+v %v", result, err)
	}
}
