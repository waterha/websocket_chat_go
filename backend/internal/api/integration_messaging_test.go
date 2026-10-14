package api

import (
	"encoding/json"
	"fmt"
	"sort"
	"sync"
	"testing"

	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/realtime"
)

func TestMessagingPersistenceOrderingAndCursors(t *testing.T) {
	services := fixture(t, false)
	ctx, chat := t.Context(), services.Chat
	alice, _ := account(t, services, "alice")
	bob, _ := account(t, services, "bob")
	charlie, _ := account(t, services, "charlie")
	outsider, _ := account(t, services, "outsider")
	direct, err := chat.Create(ctx, alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	same, err := chat.Create(ctx, bob.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{alice.ID}})
	if err != nil || same != direct {
		t.Fatalf("direct pair not unique: %s %v", same, err)
	}
	group, err := chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "team", MemberIDs: []string{bob.ID, charlie.ID}})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	results := make(chan model.Message, 80)
	failures := make(chan error, 80)
	for index := 0; index < 80; index++ {
		wait.Add(1)
		go func(index int) {
			defer wait.Done()
			sender := alice.ID
			if index%2 == 1 {
				sender = bob.ID
			}
			message, _, err := chat.Send(ctx, sender, model.SendInput{ConversationID: group, ClientMsgID: fmt.Sprintf("parallel-%03d", index), Content: "ordered"})
			if err != nil {
				failures <- err
			} else {
				results <- message
			}
		}(index)
	}
	wait.Wait()
	close(failures)
	close(results)
	for err := range failures {
		t.Error(err)
	}
	sequence := []int{}
	for message := range results {
		sequence = append(sequence, int(message.Seq))
	}
	sort.Ints(sequence)
	if len(sequence) != 80 {
		t.Fatalf("missing messages: %d", len(sequence))
	}
	for index, seq := range sequence {
		if seq != index+1 {
			t.Fatalf("sequence gap at %d: %d", index, seq)
		}
	}
	input := model.SendInput{ConversationID: group, ClientMsgID: "same-retry-id", Content: "idempotent"}
	duplicates := make(chan model.Message, 12)
	for index := 0; index < 12; index++ {
		wait.Add(1)
		go func() {
			defer wait.Done()
			item, _, err := chat.Send(ctx, alice.ID, input)
			if err != nil {
				t.Error(err)
			} else {
				duplicates <- item
			}
		}()
	}
	wait.Wait()
	close(duplicates)
	var stable string
	for item := range duplicates {
		if stable == "" {
			stable = item.ID
		}
		if item.ID != stable || item.Seq != 81 {
			t.Fatal("duplicate send inserted twice")
		}
	}
	input.Content = "different payload"
	_, _, err = chat.Send(ctx, alice.ID, input)
	status(t, err, 409)
	input.Content = "idempotent"
	input.ConversationID = direct
	_, _, err = chat.Send(ctx, alice.ID, input)
	status(t, err, 409)
	for index := 0; index < 125; index++ {
		_, _, err := chat.Send(ctx, alice.ID, model.SendInput{ConversationID: group, ClientMsgID: fmt.Sprintf("offline-%03d", index), Content: "offline"})
		if err != nil {
			t.Fatal(err)
		}
	}
	var after int64
	pages := 0
	for {
		page, err := chat.Sync(ctx, bob.ID, group, after, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, item := range page.Messages {
			if item.Seq != after+1 {
				t.Fatal("roaming gap")
			}
			after = item.Seq
		}
		if page.NextSeq != after || page.LastSeq != 206 {
			t.Fatal("bad sync cursor", page)
		}
		pages++
		if !page.HasMore {
			break
		}
	}
	if after != 206 || pages != 3 {
		t.Fatalf("offline backlog truncated: %d / %d", after, pages)
	}
	history, err := chat.History(ctx, bob.ID, group, 50, 10)
	if err != nil || len(history) != 10 || history[0].Seq != 40 || history[9].Seq != 49 {
		t.Fatalf("bad history: %v %v", history, err)
	}
	_, _, err = chat.Send(ctx, outsider.ID, model.SendInput{ConversationID: group, ClientMsgID: "forbidden-id", Content: "forbidden"})
	status(t, err, 404)
	_, err = chat.Sync(ctx, outsider.ID, group, 0, 100)
	status(t, err, 404)
	_, err = chat.History(ctx, outsider.ID, group, 50, 10)
	status(t, err, 404)
	_, err = chat.MarkRead(ctx, outsider.ID, group, 1)
	status(t, err, 404)
	_, err = chat.MarkReceived(ctx, outsider.ID, group, "phone", 1)
	status(t, err, 404)
	_, err = chat.Sync(ctx, bob.ID, group, 207, 100)
	status(t, err, 400)
	receipt, err := chat.MarkRead(ctx, bob.ID, group, 50)
	if err != nil || receipt.ReadAt == nil {
		t.Fatalf("read failed: %v", err)
	}
	older, err := chat.MarkRead(ctx, bob.ID, group, 10)
	if err != nil || older.Seq != 50 || !older.ReadAt.Equal(*receipt.ReadAt) {
		t.Fatal("read cursor moved backwards")
	}
	_, err = chat.MarkRead(ctx, bob.ID, group, 207)
	status(t, err, 400)
	for _, cursor := range []struct {
		device string
		seq    int64
	}{{"phone", 60}, {"desktop", 15}, {"phone", 10}} {
		if _, err := chat.MarkReceived(ctx, bob.ID, group, cursor.device, cursor.seq); err != nil {
			t.Fatal(err)
		}
	}
	for device, expected := range map[string]int64{"phone": 60, "desktop": 15, "new": 0} {
		cursors, err := chat.Cursors(ctx, bob.ID, device)
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, cursor := range cursors {
			if cursor.ConversationID == group {
				found = true
				if cursor.ReceivedSeq != expected {
					t.Fatal("device cursor mixed")
				}
			}
		}
		if !found {
			t.Fatal("missing cursor")
		}
	}
	list, err := chat.Conversations(ctx, bob.ID)
	if err != nil {
		t.Fatal(err)
	}
	for _, conversation := range list {
		if conversation.ID == group && conversation.LastReadSeq != 50 {
			t.Fatal("received cursor marked messages read")
		}
	}

	aliceSocket, _ := services.Hub.Register(ctx, alice.ID, "phone", "session")
	bobSocket, _ := services.Hub.Register(ctx, bob.ID, "phone", "session")
	charlieSocket, _ := services.Hub.Register(ctx, charlie.ID, "phone", "session")
	outsiderSocket, _ := services.Hub.Register(ctx, outsider.ID, "phone", "session")
	dispatcher := &realtime.Dispatcher{Store: services.Store, Hub: services.Hub}
	for index := 0; index < 3; index++ {
		if err := dispatcher.Drain(ctx); err != nil {
			t.Fatal(err)
		}
	}
	for _, client := range []*realtime.Client{aliceSocket, bobSocket, charlieSocket} {
		last, count, receipts := int64(0), 0, 0
		for len(client.Outbound) > 0 {
			event := <-client.Outbound
			if event.Event == "message.created" {
				payload, _ := json.Marshal(event.Data)
				var message model.Message
				_ = json.Unmarshal(payload, &message)
				if message.Seq != last+1 {
					t.Fatal("outbox changed message order")
				}
				last = message.Seq
				count++
			}
			if event.Event == "conversation.read" {
				receipts++
			}
		}
		if count != 206 || receipts != 1 {
			t.Fatalf("fanout mismatch: %d messages, %d receipts", count, receipts)
		}
	}
	if len(outsiderSocket.Outbound) != 0 {
		t.Fatal("outsider received group events")
	}
	late, err := services.Store.DB.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer late.Rollback(ctx)
	query := `INSERT INTO message_outbox(conversation_id,recipients,payload) VALUES($1,$2,'{"event":"probe"}')`
	if _, err = late.Exec(ctx, query, direct, []string{alice.ID}); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Store.DB.Exec(ctx, query, group, []string{alice.ID}); err != nil {
		t.Fatal(err)
	}
	if err = dispatcher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if err = late.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if err = dispatcher.Drain(ctx); err != nil {
		t.Fatal(err)
	}
	if len(aliceSocket.Outbound) != 2 {
		t.Fatal("late committed lower outbox ID skipped")
	}
	if err = services.Store.Migrate(ctx, services.Config.MigrationsDir); err != nil {
		t.Fatal("migration not repeatable", err)
	}
}
