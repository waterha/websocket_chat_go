package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"websocket_chat_go/backend/internal/logic"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

func TestWebsocketReceiveCompensationWithoutRead(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, _ := account(t, services, "delivery-alice")
	bob, token := account(t, services, "delivery-bob")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "delivery", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	receiver := dial(t, server, token, "desktop")
	if _, _, err = services.Chat.Send(t.Context(), alice.ID, model.SendInput{ConversationID: group, ClientMsgID: "dropped-last-push", Content: "must arrive"}); err != nil {
		t.Fatal(err)
	}
	reconciler := &realtime.Reconciler{Store: services.Store, Hub: services.Hub}
	for attempt := 0; attempt < 2; attempt++ {
		if err = reconciler.Retry(t.Context()); err != nil {
			t.Fatal(err)
		}
		event := await(t, receiver, "message.sync_required", "")
		var hint struct {
			Cursors []model.DeviceCursor `json:"cursors"`
		}
		if err = json.Unmarshal(event.Data, &hint); err != nil || len(hint.Cursors) != 1 || hint.Cursors[0].LastSeq != 1 || hint.Cursors[0].ReceivedSeq != 0 {
			t.Fatalf("missing delivery retry: %s %v", event.Data, err)
		}
	}
	payload := httpCall(t, server, token, http.MethodGet, "/api/v1/conversations/"+group+"/sync?after_seq=0", nil, 200)
	var page model.SyncPage
	if err = json.Unmarshal(payload, &page); err != nil || len(page.Messages) != 1 || page.NextSeq != 1 {
		t.Fatalf("failed to recover dropped push: %s %v", payload, err)
	}
	commandTo(t, receiver, "message.received", "delivery-confirm", map[string]any{"conversation_id": group, "seq": page.NextSeq})
	confirmation := await(t, receiver, "message.received.ack", "delivery-confirm")
	var cursor model.DeviceCursor
	if err = json.Unmarshal(confirmation.Data, &cursor); err != nil || cursor.ReceivedSeq != 1 {
		t.Fatalf("receive confirmation failed: %s %v", confirmation.Data, err)
	}
	if err = reconciler.Retry(t.Context()); err != nil {
		t.Fatal(err)
	}
	commandTo(t, receiver, "ping", "no-more-retries", nil)
	timer := time.NewTimer(4 * time.Second)
	defer timer.Stop()
	for {
		var event wireEvent
		select {
		case event = <-receiver.events:
		case <-receiver.done:
			t.Fatal("connection closed after receive confirmation")
		case <-timer.C:
			t.Fatal("timeout waiting for delivery retry barrier")
		}
		if event.Event == "message.sync_required" {
			t.Fatal("confirmed delivery still retried")
		}
		if event.Event == "pong" && event.RequestID == "no-more-retries" {
			break
		}
	}
	var readSeq int64
	if err = services.Store.DB.QueryRow(t.Context(), "SELECT last_read_seq FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", group, bob.ID).Scan(&readSeq); err != nil || readSeq != 0 {
		t.Fatalf("delivery changed read cursor: %d %v", readSeq, err)
	}
}

func TestThousandMemberDeliveryRecovery(t *testing.T) {
	services := fixture(t, false)
	ctx := t.Context()
	_, err := services.Store.DB.Exec(ctx, `INSERT INTO users(id,email,name,password_hash)
		SELECT 'member-'||number, 'member-'||number||'@example.test', 'member-'||number, 'unused'
		FROM generate_series(0,1000) AS number`)
	if err != nil {
		t.Fatal(err)
	}
	memberIDs := make([]string, logic.MaxGroupMembers-1)
	for index := range memberIDs {
		memberIDs[index] = fmt.Sprintf("member-%d", index+1)
	}
	group, err := services.Chat.Create(ctx, "member-0", model.CreateInput{Kind: "group", Name: "thousand", MemberIDs: memberIDs})
	if err != nil {
		t.Fatal(err)
	}
	_, err = services.Chat.Create(ctx, "member-0", model.CreateInput{Kind: "group", Name: "too many", MemberIDs: append(memberIDs, "member-1000")})
	status(t, err, 400)
	people, err := services.Auth.Users(ctx, "member-0")
	if err != nil || len(people) != 1000 {
		t.Fatalf("directory truncated: %d %v", len(people), err)
	}
	services.Hub.Close()
	services.Hub = realtime.NewHub(4)
	clients := make([]*realtime.Client, 1000)
	for index := 0; index < len(clients)-1; index++ {
		clients[index], err = services.Hub.Register(ctx, fmt.Sprintf("member-%d", index), "desktop", "session")
		if err != nil {
			t.Fatal(err)
		}
	}
	outsider, _ := services.Hub.Register(ctx, "member-1000", "desktop", "session")
	slow := clients[1]
	for index := 0; index < 4; index++ {
		slow.Reply(model.Event{Event: "occupied"})
	}
	for index := 1; index <= 3; index++ {
		if _, _, err := services.Chat.Send(ctx, "member-0", model.SendInput{ConversationID: group, ClientMsgID: fmt.Sprintf("delivery-%d", index), Content: "reliable"}); err != nil {
			t.Fatal(err)
		}
	}
	dispatcher := &realtime.Dispatcher{Store: services.Store, Hub: services.Hub}
	if err = dispatcher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if slow.Context.Err() == nil {
		t.Fatal("slow consumer was not disconnected")
	}
	if len(outsider.Outbound) != 0 {
		t.Fatal("non-member received a group message")
	}
	for index, client := range clients {
		if client == nil || index == 1 {
			continue
		}
		if len(client.Outbound) != 4 {
			t.Fatalf("fanout omitted member %d: %d events", index, len(client.Outbound))
		}
		for len(client.Outbound) > 0 {
			<-client.Outbound
		}
	}
	for _, index := range []int{1, 999} {
		clients[index], err = services.Hub.Register(ctx, fmt.Sprintf("member-%d", index), "desktop", "session")
		if err != nil {
			t.Fatal(err)
		}
	}
	services.Hub.Unregister(slow)
	reconciler := &realtime.Reconciler{Store: services.Store, Hub: services.Hub}
	assertHints := func(first int) {
		t.Helper()
		if err := reconciler.Retry(ctx); err != nil {
			t.Fatal(err)
		}
		for index, client := range clients {
			if index < first {
				if len(client.Outbound) != 0 {
					t.Fatalf("confirmed member %d was retried", index)
				}
				continue
			}
			if len(client.Outbound) != 1 {
				t.Fatalf("unconfirmed member %d missing retry", index)
			}
			event := <-client.Outbound
			cursors := event.Data.(map[string]any)["cursors"].([]model.DeviceCursor)
			if event.Event != "message.sync_required" || len(cursors) != 1 || cursors[0].ConversationID != group || cursors[0].LastSeq != 3 {
				t.Fatalf("incorrect retry: %+v", event)
			}
		}
		if len(outsider.Outbound) != 0 {
			t.Fatal("outsider received retry")
		}
	}
	assertHints(0)
	assertHints(0)
	for index := range clients {
		userID := fmt.Sprintf("member-%d", index)
		page, err := services.Chat.Sync(ctx, userID, group, 0, 100)
		if err != nil || len(page.Messages) != 3 || page.NextSeq != 3 || page.HasMore {
			t.Fatalf("member %d cannot recover: %+v %v", index, page, err)
		}
		for offset, message := range page.Messages {
			if message.Seq != int64(offset+1) {
				t.Fatal("recovered messages are not contiguous")
			}
		}
		if index < 500 {
			if _, err = services.Chat.MarkReceived(ctx, userID, group, "desktop", 3); err != nil {
				t.Fatal(err)
			}
		}
	}
	assertHints(500)
	for index := 500; index < len(clients); index++ {
		if _, err = services.Chat.MarkReceived(ctx, fmt.Sprintf("member-%d", index), group, "desktop", 3); err != nil {
			t.Fatal(err)
		}
	}
	assertHints(1000)
	var readMembers int
	if err = services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND last_read_seq<>0", group).Scan(&readMembers); err != nil || readMembers != 0 {
		t.Fatalf("receive ACK changed read state: %d %v", readMembers, err)
	}
	phone, err := services.Hub.Register(ctx, "member-0", "phone", "session")
	if err != nil {
		t.Fatal(err)
	}
	if err = reconciler.Retry(ctx); err != nil || len(phone.Outbound) != 1 || len(clients[0].Outbound) != 0 {
		t.Fatalf("device delivery cursors mixed: %v", err)
	}
	<-phone.Outbound
	if _, err = services.Chat.MarkReceived(ctx, "member-0", group, "phone", 3); err != nil {
		t.Fatal(err)
	}
	services.Hub.Close()
	services.Hub = realtime.NewHub(4)
	reconciler.Hub = services.Hub
	reconnected, _ := services.Hub.Register(ctx, "member-0", "desktop", "session")
	if err = reconciler.Retry(ctx); err != nil || len(reconnected.Outbound) != 0 {
		t.Fatalf("confirmed cursor lost after hub restart: %v", err)
	}
	if _, _, err = services.Chat.Send(ctx, "member-0", model.SendInput{ConversationID: group, ClientMsgID: "after-restart", Content: "final message"}); err != nil {
		t.Fatal(err)
	}
	if err = reconciler.Retry(ctx); err != nil || len(reconnected.Outbound) != 1 {
		t.Fatalf("message without any outbox push cannot recover: %v", err)
	}
}
