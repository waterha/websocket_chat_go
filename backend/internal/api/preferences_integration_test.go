package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestConversationPreferencesArePrivatePerMember(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "prefs", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	// Alice pins and mutes; the response reflects both.
	raw := httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/preferences", map[string]bool{"pinned": true, "muted": true}, 200)
	var result model.Preferences
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Pinned || !result.Muted || result.ConversationID != group {
		t.Fatalf("unexpected preferences: %+v", result)
	}

	// Bob's view is untouched: preferences are private.
	bobList := httpCall(t, server, bobToken, "GET", "/api/v1/conversations", nil, 200)
	var bobConversations []model.Conversation
	if err := json.Unmarshal(bobList, &bobConversations); err != nil || len(bobConversations) != 1 {
		t.Fatalf("unexpected bob list: %+v %v", bobConversations, err)
	}
	if bobConversations[0].Pinned || bobConversations[0].Muted {
		t.Fatalf("bob inherited alice's preferences: %+v", bobConversations[0])
	}

	// Alice sees her own preferences.
	aliceList := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var aliceConversations []model.Conversation
	if err := json.Unmarshal(aliceList, &aliceConversations); err != nil || len(aliceConversations) != 1 {
		t.Fatalf("unexpected alice list: %+v %v", aliceConversations, err)
	}
	if !aliceConversations[0].Pinned || !aliceConversations[0].Muted {
		t.Fatalf("alice lost her preferences: %+v", aliceConversations[0])
	}

	// An omitted field is left unchanged rather than reset to false.
	raw = httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/preferences", map[string]bool{"muted": false}, 200)
	var updated model.Preferences
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Muted || !updated.Pinned {
		t.Fatalf("partial update changed unrelated fields: %+v", updated)
	}

	// Clearing is explicit.
	raw = httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/preferences", map[string]bool{"pinned": false}, 200)
	var cleared model.Preferences
	if err := json.Unmarshal(raw, &cleared); err != nil {
		t.Fatal(err)
	}
	if cleared.Pinned {
		t.Fatalf("clearing failed: %+v", cleared)
	}

	// Pinning several conversations orders them by pin time, newest pin first.
	second, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "second-prefs"})
	if err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/preferences", map[string]bool{"pinned": true}, 200)
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+second+"/preferences", map[string]bool{"pinned": true}, 200)
	aliceList = httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	if err := json.Unmarshal(aliceList, &aliceConversations); err != nil {
		t.Fatal(err)
	}
	if len(aliceConversations) != 2 || aliceConversations[0].ID != second {
		t.Fatalf("the most recently pinned conversation should sort first: %+v", aliceConversations)
	}

	// A non-member and an unknown conversation are both not found.
	_, charlieToken := account(t, services, "charlie")
	httpCall(t, server, charlieToken, "PATCH", "/api/v1/conversations/"+group+"/preferences", map[string]bool{"pinned": true}, 404)
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/missing/preferences", map[string]bool{"pinned": true}, 404)
}
