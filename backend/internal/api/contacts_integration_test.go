package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

// befriend walks the request flow so tests can start from a friendship.
func befriend(t *testing.T, services *svc.ServiceContext, requester, recipient model.User) {
	t.Helper()
	request, err := services.Auth.CreateFriendRequest(t.Context(), requester.ID, model.AddFriendInput{UserID: recipient.ID, Message: "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err = services.Auth.DecideFriendRequest(t.Context(), recipient.ID, request.ID, "accepted"); err != nil {
		t.Fatal(err)
	}
}

func TestFriendRemarkAndDeletion(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	befriend(t, services, alice, bob)

	// The remark is private to the caller: alice names bob, bob sees nothing.
	raw := httpCall(t, server, aliceToken, "PATCH", "/api/v1/friends/"+bob.ID, map[string]string{"remark": "老鲍"}, 200)
	var friend model.Friend
	if err := json.Unmarshal(raw, &friend); err != nil {
		t.Fatal(err)
	}
	if friend.Remark != "老鲍" || friend.ID != bob.ID {
		t.Fatalf("unexpected friend payload: %+v", friend)
	}
	list := httpCall(t, server, bobToken, "GET", "/api/v1/friends", nil, 200)
	var bobFriends []model.Friend
	if err := json.Unmarshal(list, &bobFriends); err != nil {
		t.Fatal(err)
	}
	if len(bobFriends) != 1 || bobFriends[0].Remark != "" {
		t.Fatalf("remark leaked to the other side: %+v", bobFriends)
	}

	// Deleting ends the friendship in both directions but stays idempotent.
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/friends/"+bob.ID, nil, 200)
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/friends/"+bob.ID, nil, 404)
	list = httpCall(t, server, bobToken, "GET", "/api/v1/friends", nil, 200)
	if err := json.Unmarshal(list, &bobFriends); err != nil || len(bobFriends) != 0 {
		t.Fatalf("friendship survived deletion: %+v %v", bobFriends, err)
	}
	// A deleted friendship can be restored through a fresh request.
	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", map[string]string{"user_id": bob.ID, "message": "again"}, 201)
}

func TestBlockingGatesContact(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")

	httpCall(t, server, aliceToken, "POST", "/api/v1/blocks", map[string]string{"user_id": bob.ID}, 201)

	// A blocked user cannot reach the blocker: neither a request nor a room.
	httpCall(t, server, bobToken, "POST", "/api/v1/friends", map[string]string{"user_id": alice.ID}, 403)
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations", model.CreateInput{Kind: "direct", MemberIDs: []string{alice.ID}}, 403)
	// The blocker cannot open the room either; the block cuts both ways.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations", model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}}, 400)

	// Unblocking restores the ordinary flow.
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/blocks/"+bob.ID, nil, 200)
	raw := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations", model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}}, 201)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		t.Fatal(err)
	}

	// The list reflects exactly one block for one block call.
	httpCall(t, server, aliceToken, "POST", "/api/v1/blocks", map[string]string{"user_id": bob.ID}, 201)
	blocks := httpCall(t, server, aliceToken, "GET", "/api/v1/blocks", nil, 200)
	var entries []model.BlockEntry
	if err := json.Unmarshal(blocks, &entries); err != nil || len(entries) != 1 {
		t.Fatalf("unexpected block list: %+v %v", entries, err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/blocks", map[string]string{"user_id": alice.ID}, 400)
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/blocks/"+bob.ID, nil, 200)
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/blocks/"+bob.ID, nil, 404)
}

func TestBlockingSilencesDirectMessages(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, _ := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	befriend(t, services, alice, bob)
	direct, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	httpCall(t, server, bobToken, "POST", "/api/v1/blocks", map[string]string{"user_id": alice.ID}, 201)
	// Alice's messages stop at the door while her own view is unchanged.
	_, _, err = services.Chat.Send(ctx, alice.ID, model.SendInput{ConversationID: direct, ClientMsgID: "blocked-1", Content: "hello?"})
	status(t, err, 403)
	// Bob still speaks into the conversation he muted.
	if _, _, err = services.Chat.Send(ctx, bob.ID, model.SendInput{ConversationID: direct, ClientMsgID: "muted-send-1", Content: "silence"}); err != nil {
		t.Fatal(err)
	}
	// A group is every member's room: blocking does not apply there.
	carol, _ := account(t, services, "carol")
	group, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "room", MemberIDs: []string{bob.ID, carol.ID}})
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err = services.Chat.Send(ctx, alice.ID, model.SendInput{ConversationID: group, ClientMsgID: "group-send-1", Content: "still talkative"}); err != nil {
		t.Fatal(err)
	}
}
