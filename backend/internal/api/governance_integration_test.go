package api

import (
	"encoding/json"
	"net/http/httptest"
	"strings"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestMemberAlias(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, carolToken := account(t, services, "carol")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "team", MemberIDs: []string{bob.ID, carol.ID}})
	if err != nil {
		t.Fatal(err)
	}

	// A member renames themselves; an admin renames others; a plain member
	// cannot rename anyone else.
	httpCall(t, server, bobToken, "PATCH", "/api/v1/conversations/"+group+"/members/"+bob.ID+"/alias", map[string]string{"alias": "阿鲍"}, 200)
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/members/"+bob.ID+"/alias", map[string]string{"alias": "鲍工"}, 200)
	httpCall(t, server, carolToken, "PATCH", "/api/v1/conversations/"+group+"/members/"+bob.ID+"/alias", map[string]string{"alias": "nope"}, 403)
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/members/"+bob.ID+"/alias", map[string]string{"alias": strings.Repeat("长", 33)}, 400)

	list := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var conversations []model.Conversation
	if err := json.Unmarshal(list, &conversations); err != nil {
		t.Fatal(err)
	}
	for _, member := range conversations[0].Members {
		if member.ID == bob.ID && member.Alias != "鲍工" {
			t.Fatalf("alias not stored: %+v", member)
		}
	}
}

func TestMemberAndGroupMute(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, carolToken := account(t, services, "carol")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "quiet", MemberIDs: []string{bob.ID, carol.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if err = services.Chat.SetRole(ctx, alice.ID, group, carol.ID, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}

	// Permission boundaries around muting.
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/members/"+alice.ID+"/mute", map[string]int{"minutes": 10}, 403)
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+group+"/members/"+alice.ID+"/mute", map[string]int{"minutes": 10}, 403)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/members/"+bob.ID+"/mute", map[string]int{"minutes": 60}, 200)
	// A muted member cannot speak, an admin still can.
	_, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: group, ClientMsgID: "muted-send", Content: "let me in"})
	status(t, err, 403)
	if _, _, err = services.Chat.Send(ctx, carol.ID, model.SendInput{ConversationID: group, ClientMsgID: "admin-send", Content: "admins speak"}); err != nil {
		t.Fatal(err)
	}

	// Group-wide mute silences plain members but not admins or the owner.
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/settings", map[string]any{"all_muted": true}, 200)
	_, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: group, ClientMsgID: "allmuted-send", Content: "still quiet"})
	status(t, err, 403)
	if _, _, err = services.Chat.Send(ctx, carol.ID, model.SendInput{ConversationID: group, ClientMsgID: "admin-send-2", Content: "admin unaffected"}); err != nil {
		t.Fatal(err)
	}

	// Lifting both mutes restores the member's voice.
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/settings", map[string]any{"all_muted": false}, 200)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/members/"+bob.ID+"/mute", map[string]int{"minutes": 0}, 200)
	if _, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: group, ClientMsgID: "unmuted-send", Content: "finally"}); err != nil {
		t.Fatal(err)
	}
}

func TestDismissGroup(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "doomed", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/dismiss", nil, 403)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/dismiss", nil, 200)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/dismiss", nil, 409)
	// History stays readable, but nobody may speak.
	_, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: group, ClientMsgID: "after-dismiss", Content: "echo"})
	status(t, err, 403)
	history := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+group+"/messages", nil, 200)
	var messages []model.Message
	if err := json.Unmarshal(history, &messages); err != nil {
		t.Fatal(err)
	}
	if len(messages) == 0 {
		t.Fatal("history vanished after dismissal")
	}
}

func TestJoinApprovalFlow(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	charlie, charlieToken := account(t, services, "charlie")
	dave, _ := account(t, services, "dave")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "gated", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/settings", map[string]any{"join_approval": true}, 200)

	// A member's invite becomes a pending request; the invitee is not added yet.
	raw := httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/members", model.AddMembersInput{MemberIDs: []string{charlie.ID}}, 200)
	var result model.AddMembersResult
	if err := json.Unmarshal(raw, &result); err != nil {
		t.Fatal(err)
	}
	if !result.Pending {
		t.Fatalf("invite should be pending: %+v", result)
	}
	httpCall(t, server, charlieToken, "GET", "/api/v1/conversations", nil, 200)

	requests := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/join-requests", nil, 200)
	var pending []model.JoinRequest
	if err := json.Unmarshal(requests, &pending); err != nil || len(pending) != 1 {
		t.Fatalf("unexpected pending requests: %+v %v", pending, err)
	}
	// Plain members and non-members may not decide.
	httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+group+"/join-requests", nil, 403)
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/join-requests/"+pending[0].ID+"/accept", nil, 403)

	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/join-requests/"+pending[0].ID+"/accept", nil, 200)
	members := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var conversations []model.Conversation
	if err := json.Unmarshal(members, &conversations); err != nil {
		t.Fatal(err)
	}
	if countMembers(conversations[0]) != 3 {
		t.Fatalf("charlie did not join: %+v", conversations[0].Members)
	}

	// A rejected request keeps the invitee out.
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/members", model.AddMembersInput{MemberIDs: []string{dave.ID}}, 200)
	requests = httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/join-requests", nil, 200)
	if err := json.Unmarshal(requests, &pending); err != nil || len(pending) != 1 {
		t.Fatalf("unexpected second round: %+v %v", pending, err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/join-requests/"+pending[0].ID+"/reject", nil, 200)
	members = httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	if err := json.Unmarshal(members, &conversations); err != nil {
		t.Fatal(err)
	}
	if countMembers(conversations[0]) != 3 {
		t.Fatalf("dave joined without approval: %+v", conversations[0].Members)
	}
	// The owner still adds directly, without an approval round trip.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/members", model.AddMembersInput{MemberIDs: []string{dave.ID}}, 200)
	members = httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	if err := json.Unmarshal(members, &conversations); err != nil {
		t.Fatal(err)
	}
	if countMembers(conversations[0]) != 4 {
		t.Fatalf("owner invite should not be pending: %+v", conversations[0].Members)
	}
}

func countMembers(conversation model.Conversation) int {
	return len(conversation.Members)
}

func TestDeleteConversationRestoresOnMessage(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	direct, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: direct, ClientMsgID: "before-delete", Content: "hello"}); err != nil {
		t.Fatal(err)
	}

	httpCall(t, server, aliceToken, "DELETE", "/api/v1/conversations/"+direct, nil, 200)
	aliceList := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var conversations []model.Conversation
	if err := json.Unmarshal(aliceList, &conversations); err != nil || len(conversations) != 0 {
		t.Fatalf("deleted conversation still listed: %+v %v", conversations, err)
	}
	// Bob's view is untouched.
	bobList := httpCall(t, server, bobToken, "GET", "/api/v1/conversations", nil, 200)
	if err := json.Unmarshal(bobList, &conversations); err != nil || len(conversations) != 1 {
		t.Fatalf("bob lost his conversation: %+v %v", conversations, err)
	}
	// The next message reopens the conversation for alice.
	if _, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: direct, ClientMsgID: "after-delete", Content: "knock knock"}); err != nil {
		t.Fatal(err)
	}
	aliceList = httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	if err := json.Unmarshal(aliceList, &conversations); err != nil || len(conversations) != 1 {
		t.Fatalf("message did not restore the conversation: %+v %v", conversations, err)
	}
}

func TestMessageReactions(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "react", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	message, _, err := services.Chat.Send(ctx, alice.ID, model.SendInput{ConversationID: group, ClientMsgID: "react-target", Content: "say something"})
	if err != nil {
		t.Fatal(err)
	}
	path := "/api/v1/conversations/" + group + "/messages/" + message.ID + "/reactions"

	raw := httpCall(t, server, aliceToken, "POST", path, model.ReactionInput{Emoji: "👍"}, 200)
	var updated model.Message
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Reactions) != 1 || updated.Reactions[0].Count != 1 || updated.Reactions[0].Emoji != "👍" {
		t.Fatalf("unexpected reactions: %+v", updated.Reactions)
	}
	// A second member joins the same emoji, and the aggregate follows.
	raw = httpCall(t, server, bobToken, "POST", path, model.ReactionInput{Emoji: "👍"}, 200)
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Reactions) != 1 || updated.Reactions[0].Count != 2 || len(updated.Reactions[0].Users) != 2 {
		t.Fatalf("aggregate not merged: %+v", updated.Reactions)
	}
	// Removing one of two keeps the reaction with one user left.
	raw = httpCall(t, server, aliceToken, "DELETE", path, model.ReactionInput{Emoji: "👍"}, 200)
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Reactions) != 1 || updated.Reactions[0].Count != 1 {
		t.Fatalf("unreact did not shrink the aggregate: %+v", updated.Reactions)
	}
	// A second emoji from the same user is a separate row.
	httpCall(t, server, bobToken, "POST", path, model.ReactionInput{Emoji: "🎉"}, 200)
	raw = httpCall(t, server, bobToken, "POST", path, model.ReactionInput{Emoji: "🎉"}, 200)
	if err := json.Unmarshal(raw, &updated); err != nil {
		t.Fatal(err)
	}
	if len(updated.Reactions) != 2 {
		t.Fatalf("expected two emoji groups: %+v", updated.Reactions)
	}

	// Invalid input and missing or recalled messages are refused.
	httpCall(t, server, bobToken, "POST", path, model.ReactionInput{Emoji: strings.Repeat("长", 17)}, 400)
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/messages/missing/reactions", model.ReactionInput{Emoji: "👍"}, 404)
	if _, err = services.Chat.Recall(ctx, alice.ID, group, message.Seq); err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, bobToken, "POST", path, model.ReactionInput{Emoji: "👍"}, 409)
}

func TestTypingRelay(t *testing.T) {
	services := fixture(t, true)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	_, carolToken := account(t, services, "carol")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "typing", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	alicePeer := dial(t, server, aliceToken, "alice-device")
	bobPeer := dial(t, server, bobToken, "bob-device")
	carolPeer := dial(t, server, carolToken, "carol-device")

	commandTo(t, alicePeer, "typing.send", "typing-1", map[string]string{"conversation_id": group})
	// Members receive the ephemeral relay with an ack for the sender.
	await(t, bobPeer, "typing", "")
	ack := await(t, alicePeer, "typing.ack", "typing-1")
	var payload struct {
		ConversationID string `json:"conversation_id"`
	}
	if err := json.Unmarshal(ack.Data, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.ConversationID != group {
		t.Fatalf("ack names the wrong conversation: %+v", payload)
	}
	// A non-member is refused and never relays.
	commandTo(t, carolPeer, "typing.send", "typing-2", map[string]string{"conversation_id": group})
	rejected := await(t, carolPeer, "error", "typing-2")
	if rejected.Error == nil || rejected.Error.Code != "not_found" {
		t.Fatalf("expected not_found for a non-member: %+v", rejected.Error)
	}
	noMessages(t, bobPeer)
}
