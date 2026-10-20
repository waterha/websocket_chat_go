package api

import (
	"encoding/json"
	"testing"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
)

// TestThreadGroupsRepliesUnderOneRoot is the core thread behaviour: a reply
// starts a window, a reply to that reply joins the same window, and the window
// returns every reply oldest first.
func TestThreadGroupsRepliesUnderOneRoot(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, carolToken, group := replyFixture(t)

	root := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "thread-root-0001", "content": "你好"}, 201)
	if root.ThreadRootID != "" {
		t.Fatalf("a message with no reply must not be filed into a window: %+v", root)
	}

	// Two members reply to the root, and one of them replies to a reply.
	first := sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "thread-r1-0001", "content": "你好呀", "reply_to_id": root.ID}, 201)
	second := sendMessage(t, server, carolToken, group, map[string]any{"client_msg_id": "thread-r2-0001", "content": "早上好", "reply_to_id": root.ID}, 201)
	// A reply to a reply must land in the original window, not a nested one.
	nested := sendMessage(t, server, bobToken, group, map[string]any{"client_msg_id": "thread-r3-0001", "content": "对，早上好", "reply_to_id": first.ID}, 201)

	if first.ThreadRootID != root.ID || second.ThreadRootID != root.ID {
		t.Fatalf("direct replies were not filed under the root: %q %q", first.ThreadRootID, second.ThreadRootID)
	}
	if nested.ThreadRootID != root.ID {
		t.Fatalf("a reply to a reply must join the original window, got %q", nested.ThreadRootID)
	}
	if nested.ReplyToID != first.ID {
		t.Fatalf("the quoted target must still be the immediate parent: %q", nested.ReplyToID)
	}

	// The window returns every reply of the root, oldest first.
	raw := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread", nil, 200)
	var page model.ThreadPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	if page.ReplyCount != 3 || len(page.Replies) != 3 {
		t.Fatalf("window should hold three replies: count=%d got=%d", page.ReplyCount, len(page.Replies))
	}
	if page.Root.ID != root.ID || page.Root.Content != "你好" {
		t.Fatalf("window did not return its root: %+v", page.Root)
	}
	for index, reply := range page.Replies {
		if reply.ThreadRootID != root.ID {
			t.Fatalf("reply %d was not filed under the root: %q", index, reply.ThreadRootID)
		}
	}
	if page.Replies[0].ID != first.ID || page.Replies[1].ID != second.ID || page.Replies[2].ID != nested.ID {
		t.Fatalf("replies are not oldest first: %+v", []string{page.Replies[0].ID, page.Replies[1].ID, page.Replies[2].ID})
	}

	// Opening the window from a reply must resolve to the same window rather than
	// creating a nested one.
	fromReply := httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+group+"/messages/"+nested.ID+"/thread", nil, 200)
	var nestedPage model.ThreadPage
	if err := json.Unmarshal(fromReply, &nestedPage); err != nil {
		t.Fatal(err)
	}
	if nestedPage.Root.ID != root.ID || nestedPage.ReplyCount != 3 || len(nestedPage.Replies) != 3 {
		t.Fatalf("opening from a reply must resolve to the root window: root=%q count=%d replies=%d", nestedPage.Root.ID, nestedPage.ReplyCount, len(nestedPage.Replies))
	}
}

// The main chat view must show a bounded preview, not the whole window.
func TestThreadSummaryLimitsPreview(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, carolToken, group := replyFixture(t)

	root := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "thread-sum-0001", "content": "会议几点"}, 201)
	for index := 0; index < logic.ThreadPreviewLimit+2; index++ {
		sendMessage(t, server, bobToken, group, map[string]any{
			"client_msg_id": "thread-sum-r" + itoa(int64(index)) + "-000",
			"content":       "回复 " + itoa(int64(index)),
			"reply_to_id":   root.ID,
		}, 201)
	}

	// The summary travels with the message when the conversation is read.
	raw := httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/sync?after_seq=0&limit=100", nil, 200)
	var page model.SyncPage
	if err := json.Unmarshal(raw, &page); err != nil {
		t.Fatal(err)
	}
	var found *model.Message
	for index := range page.Messages {
		if page.Messages[index].ID == root.ID {
			found = &page.Messages[index]
		}
	}
	if found == nil {
		t.Fatal("the root message was not returned")
	}
	if found.Thread == nil {
		t.Fatalf("the root carries no thread summary: %+v", found)
	}
	if found.Thread.ReplyCount != logic.ThreadPreviewLimit+2 {
		t.Fatalf("the summary must report the true reply count, got %d", found.Thread.ReplyCount)
	}
	if len(found.Thread.Preview) != logic.ThreadPreviewLimit {
		t.Fatalf("the preview must be capped at %d, got %d", logic.ThreadPreviewLimit, len(found.Thread.Preview))
	}
	// A preview shows the oldest replies, matching the window order.
	if found.Thread.Preview[0].Content != "回复 0" {
		t.Fatalf("the preview should start at the oldest reply, got %q", found.Thread.Preview[0].Content)
	}

	// A message nobody replied to carries no summary, so the client can hide the
	// "view replies" affordance.
	plain := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "thread-plain-001", "content": "没有回复"}, 201)
	synced := httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/messages?before_seq=9999&limit=100", nil, 200)
	var history []model.Message
	if err := json.Unmarshal(synced, &history); err != nil {
		t.Fatal(err)
	}
	for _, message := range history {
		if message.ID == plain.ID && message.Thread != nil {
			t.Fatalf("a message without replies must not carry a summary: %+v", message.Thread)
		}
	}
}

// Window access must be limited to conversation members and to real messages.
func TestThreadAccessAndPaging(t *testing.T) {
	services, server, _, aliceToken, _, bobToken, _, _, group := replyFixture(t)

	root := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "thread-page-0001", "content": "分页测试"}, 201)
	for index := 0; index < 5; index++ {
		sendMessage(t, server, bobToken, group, map[string]any{
			"client_msg_id": "thread-page-r" + itoa(int64(index)) + "-00",
			"content":       "第 " + itoa(int64(index)) + " 条",
			"reply_to_id":   root.ID,
		}, 201)
	}

	// Paging walks the window in order.
	first := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread?limit=2", nil, 200)
	var page model.ThreadPage
	if err := json.Unmarshal(first, &page); err != nil {
		t.Fatal(err)
	}
	if len(page.Replies) != 2 || !page.HasMore || page.NextAfter != page.Replies[1].Seq {
		t.Fatalf("first page is wrong: len=%d more=%v next=%d", len(page.Replies), page.HasMore, page.NextAfter)
	}
	second := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread?limit=2&after_seq="+itoa(page.NextAfter), nil, 200)
	var next model.ThreadPage
	if err := json.Unmarshal(second, &next); err != nil {
		t.Fatal(err)
	}
	if len(next.Replies) != 2 || next.Replies[0].Seq <= page.NextAfter {
		t.Fatalf("second page did not continue after the cursor: %+v", next.Replies)
	}
	// The reply count is the window total, not the page size.
	if next.ReplyCount != 5 {
		t.Fatalf("page must report the full reply count, got %d", next.ReplyCount)
	}

	// An outsider cannot read the window.
	_, outsiderToken := account(t, services, "thread-outsider")
	httpCall(t, server, outsiderToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread", nil, 404)
	// An unknown message and a bad limit are refused.
	httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/does-not-exist/thread", nil, 404)
	httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread?limit=0", nil, 400)
	httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+root.ID+"/thread?after_seq=-1", nil, 400)
}

// A client cannot file a reply into an unrelated window by claiming a root.
func TestThreadRootCannotBeForged(t *testing.T) {
	_, server, _, aliceToken, _, bobToken, _, _, group := replyFixture(t)

	rootA := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "forge-root-a-01", "content": "窗口 A"}, 201)
	rootB := sendMessage(t, server, aliceToken, group, map[string]any{"client_msg_id": "forge-root-b-01", "content": "窗口 B"}, 201)

	// Bob replies to A while claiming the window of B.
	forged := sendMessage(t, server, bobToken, group, map[string]any{
		"client_msg_id": "forge-reply-0001", "content": "试图混入", "reply_to_id": rootA.ID, "thread_root_id": rootB.ID,
	}, 201)
	if forged.ThreadRootID != rootA.ID {
		t.Fatalf("the server must derive the root from the quoted message, got %q", forged.ThreadRootID)
	}
	// Window B therefore holds nobody, and window A holds the reply.
	windowB := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/messages/"+rootB.ID+"/thread", nil, 200)
	var pageB model.ThreadPage
	if err := json.Unmarshal(windowB, &pageB); err != nil {
		t.Fatal(err)
	}
	if pageB.ReplyCount != 0 || len(pageB.Replies) != 0 {
		t.Fatalf("window B must stay empty: count=%d replies=%d", pageB.ReplyCount, len(pageB.Replies))
	}
}
