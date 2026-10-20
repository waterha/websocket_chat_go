package api

import (
	"context"
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

// TestReplySurvivesTheWebsocketSendPath guards the defect found while wiring the
// thread window: the client sends its stored pending-send record verbatim over
// WebSocket, and that record previously omitted reply_to_id and mentions, so a
// reply sent over the real socket silently lost its quoting while the HTTP
// fallback kept it. This test sends a reply as a WebSocket command, which is the
// path a browser actually uses.
func TestReplySurvivesTheWebsocketSendPath(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "wssend-alice")
	bob, bobToken := account(t, services, "wssend-bob")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "wssend", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}

	root := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "wssend-root-0001", "content": "你好"}, 201)

	// Bob replies over the socket, exactly as the browser does.
	peer := dial(t, server, bobToken, "wssend-device")
	commandTo(t, peer, "message.send", "wssend-reply-0001", model.SendInput{
		ConversationID: group,
		ClientMsgID:    "wssend-reply-0001",
		Content:        "你好呀",
		ReplyToID:      root.ID,
		Mentions:       []string{alice.ID},
	})
	ack := await(t, peer, "message.ack", "wssend-reply-0001")
	var reply model.Message
	if err := json.Unmarshal(ack.Data, &reply); err != nil {
		t.Fatal(err)
	}
	// The reply must carry its quoted target and its mentions, having travelled
	// over the socket rather than the HTTP endpoint.
	if reply.ReplyToID != root.ID {
		t.Fatalf("a socket reply lost its quoted target: %q", reply.ReplyToID)
	}
	if len(reply.Mentions) != 1 || reply.Mentions[0] != alice.ID {
		t.Fatalf("a socket reply lost its mentions: %+v", reply.Mentions)
	}
	// And it must be filed into the window that the reply starts.
	if reply.ThreadRootID != root.ID {
		t.Fatalf("a socket reply was not filed into the window: %q", reply.ThreadRootID)
	}

	// The window therefore contains the reply.
	raw := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread", nil, 200)
	var page model.ThreadPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if page.ReplyCount != 1 || len(page.Replies) != 1 || page.Replies[0].ReplyToID != root.ID {
		t.Fatalf("the window did not receive the socket reply: count=%d replies=%d", page.ReplyCount, len(page.Replies))
	}
	_ = bob
}

// A reply written in the window joins that window even when many people reply in
// a burst, and every reply is returned by the window in order.
func TestWindowRepliesFromManyMembers(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "burst-alice")
	members := []model.User{}
	tokens := []string{}
	for index := 0; index < 4; index++ {
		member, token := account(t, services, "burst-"+string(rune('a'+index)))
		members = append(members, member)
		tokens = append(tokens, token)
	}
	ids := []string{}
	for _, member := range members {
		ids = append(ids, member.ID)
	}
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "burst", MemberIDs: ids})
	if err != nil {
		t.Fatal(err)
	}
	root := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "burst-root-0001", "content": "大家好"}, 201)

	// Every member replies to the root concurrently, the way a group chat behaves.
	wait := make(chan error, len(members))
	for index, token := range tokens {
		go func(index int, token string) {
			_, _, err := services.Chat.Send(context.Background(), members[index].ID, model.SendInput{
				ConversationID: group,
				ClientMsgID:    "burst-reply-" + string(rune('a'+index)) + "-0001",
				Content:        "回复 " + string(rune('a'+index)),
				ReplyToID:      root.ID,
			})
			wait <- err
		}(index, token)
	}
	for range members {
		if err := <-wait; err != nil {
			t.Fatal(err)
		}
	}

	raw := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread?limit=100", nil, 200)
	var page model.ThreadPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if page.ReplyCount != len(members) || len(page.Replies) != len(members) {
		t.Fatalf("every concurrent reply must be in the window: count=%d replies=%d", page.ReplyCount, len(page.Replies))
	}
	// Window order is by conversation sequence, so it is oldest first.
	for index := 1; index < len(page.Replies); index++ {
		if page.Replies[index].Seq <= page.Replies[index-1].Seq {
			t.Fatalf("window replies are not in sequence order at %d", index)
		}
	}
	// All replies are non-empty and carry their quoting.
	for _, reply := range page.Replies {
		if reply.Content == "" || reply.ReplyToID != root.ID {
			t.Fatalf("a window reply is incomplete: %+v", reply)
		}
	}
	// The chat list preview stays bounded while the header reports the full count.
	synced := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/sync?after_seq=0&limit=100", nil, 200)
	var syncPage model.SyncPage
	if err := json.Unmarshal(synced, &syncPage); err != nil {
		t.Fatal(err)
	}
	for _, message := range syncPage.Messages {
		if message.ID == root.ID {
			if message.Thread == nil || message.Thread.ReplyCount != len(members) {
				t.Fatalf("the summary count is wrong: %+v", message.Thread)
			}
			if len(message.Thread.Preview) > 3 {
				t.Fatalf("the inline preview exceeded three replies: %d", len(message.Thread.Preview))
			}
		}
	}
}
