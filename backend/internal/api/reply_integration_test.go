package api

import (
	"encoding/json"
	"net/http/httptest"
	"strconv"
	"testing"

	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

// replyFixture creates a group owned by alice with bob and carol as members.
func replyFixture(t *testing.T) (*svc.ServiceContext, *httptest.Server, model.User, string, model.User, string, model.User, string, string) {
	t.Helper()
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, carolToken := account(t, services, "carol")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "team", MemberIDs: []string{bob.ID, carol.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return services, server, alice, aliceToken, bob, bobToken, carol, carolToken, group
}

func sendMessage(t *testing.T, server *httptest.Server, token, group string, body map[string]any, expected int) model.Message {
	t.Helper()
	raw := httpCall(t, server, token, "POST", "/api/v1/conversations/"+group+"/messages", body, expected)
	var message model.Message
	if expected == 201 || expected == 200 {
		if err := json.Unmarshal(raw, &message); err != nil {
			t.Fatal(err)
		}
	}
	return message
}

func TestReplyQuotingAndMentions(t *testing.T) {
	services, server, alice, aliceToken, _, bobToken, carol, carolToken, group := replyFixture(t)

	original := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "original-01", "content": "请确认周五的排期"}, 201)
	if original.Kind != "user" || original.Reply != nil || len(original.Mentions) != 0 {
		t.Fatalf("unexpected plain message: %+v", original)
	}

	reply := sendMessage(t, server, bobToken, group, map[string]any{
		"client_msg_id": "reply-0001", "content": "收到", "reply_to_id": original.ID, "mentions": []string{alice.ID},
	}, 201)
	if reply.ReplyToID != original.ID || reply.Reply == nil {
		t.Fatalf("reply was not quoted: %+v", reply)
	}
	if reply.Reply.Seq != original.Seq || reply.Reply.SenderID != alice.ID || reply.Reply.Excerpt != "请确认周五的排期" || reply.Reply.Recalled {
		t.Fatalf("unexpected quoted context: %+v", reply.Reply)
	}
	if len(reply.Mentions) != 1 || reply.Mentions[0] != alice.ID {
		t.Fatalf("mentions were not recorded: %+v", reply.Mentions)
	}

	// Retrying the same send is idempotent even with reply and mentions set.
	retried := sendMessage(t, server, bobToken, group, map[string]any{
		"client_msg_id": "reply-0001", "content": "收到", "reply_to_id": original.ID, "mentions": []string{alice.ID},
	}, 200)
	if retried.ID != reply.ID {
		t.Fatalf("retry created a new message: %+v", retried)
	}
	// Changing the quoted target with the same client id is a conflict.
	sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "reply-0001", "content": "收到", "reply_to_id": "", "mentions": []string{alice.ID}}, 409)
	// Changing mentions is also a conflict.
	sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "reply-0001", "content": "收到", "reply_to_id": original.ID, "mentions": []string{carol.ID}}, 409)

	// Duplicate mentions are collapsed, and non-members are refused.
	duplicated := sendMessage(t, server, carolToken, group, map[string]any{"client_msg_id": "dedupe-001", "content": "好的", "mentions": []string{alice.ID, alice.ID}}, 201)
	if len(duplicated.Mentions) != 1 {
		t.Fatalf("mentions were not deduplicated: %+v", duplicated.Mentions)
	}
	outsider, _ := account(t, services, "outsider")
	sendMessage(t, server, carolToken, group, map[string]any{"client_msg_id": "bad-00001", "content": "hi", "mentions": []string{outsider.ID}}, 400)
	// An empty mention entry is invalid.
	sendMessage(t, server, carolToken, group, map[string]any{"client_msg_id": "bad-00002", "content": "hi", "mentions": []string{""}}, 400)

	// A reply cannot quote a message from another conversation.
	other, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "other"})
	if err != nil {
		t.Fatal(err)
	}
	otherMessage := sendMessage(t, server, aliceToken, other, map[string]any{"client_msg_id": "other-0001", "content": "elsewhere"}, 201)
	sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "cross-0001", "content": "hi", "reply_to_id": otherMessage.ID}, 404)
	// A missing quoted message is not found rather than silently ignored.
	sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "cross-0002", "content": "hi", "reply_to_id": "does-not-exist"}, 404)

	// Sync reproduces the quoted context for every member.
	synced := httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/sync?after_seq=0&limit=100", nil, 200)
	var page model.SyncPage
	if err := json.Unmarshal(synced, &page); err != nil {
		t.Fatal(err)
	}
	for _, message := range page.Messages {
		if message.Seq == reply.Seq {
			if message.Reply == nil || message.Reply.Excerpt != "请确认周五的排期" {
				t.Fatalf("sync lost the quoted context: %+v", message)
			}
		}
	}
}

func TestRecalledMessageHidesContent(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, carolToken, group := replyFixture(t)
	message := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "recall-001", "content": "这条要撤回"}, 201)

	// Someone else cannot recall another member's message.
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(message.Seq)+"/recall", nil, 403)
	// A nonexistent sequence is not found.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/9999/recall", nil, 404)

	recalled := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(message.Seq)+"/recall", nil, 200)
	var result model.Message
	if err := json.Unmarshal(recalled, &result); err != nil {
		t.Fatal(err)
	}
	if result.RecalledAt == nil || result.Content != "" || result.Media != nil {
		t.Fatalf("recalled message still exposes content: %+v", result)
	}
	// The sequence is preserved so history stays contiguous.
	if result.Seq != message.Seq {
		t.Fatalf("recall changed the sequence: %d", result.Seq)
	}
	// Recalling twice is idempotent.
	again := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(message.Seq)+"/recall", nil, 200)
	var repeated model.Message
	if err := json.Unmarshal(again, &repeated); err != nil || repeated.RecalledAt == nil {
		t.Fatalf("second recall failed: %v %v", repeated, err)
	}

	// Other members see the recalled state and no body through sync.
	synced := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+group+"/sync?after_seq=0&limit=100", nil, 200)
	var page model.SyncPage
	if err := json.Unmarshal(synced, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Messages) != 1 || page.Messages[0].RecalledAt == nil || page.Messages[0].Content != "" {
		t.Fatalf("sync exposed a recalled body: %+v", page.Messages)
	}

	// A reply that quoted the message keeps its excerpt only before recall.
	reply := sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "after-recall-01", "content": "已撤回?", "reply_to_id": message.ID}, 201)
	if reply.Reply == nil || !reply.Reply.Recalled || reply.Reply.Excerpt != "" {
		t.Fatalf("quote did not reflect the recall: %+v", reply.Reply)
	}

	// Group moderators may recall another member's message.
	moderated := sendMessage(t, server, carolToken, group, map[string]any{"client_msg_id": "moderate-01", "content": "请管理员处理"}, 201)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/messages/"+itoa(moderated.Seq)+"/recall", nil, 200)
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }
