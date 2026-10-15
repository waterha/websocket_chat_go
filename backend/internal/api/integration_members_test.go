package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestGroupMemberInvitationHTTPAndWebsocket(t *testing.T) {
	services := fixture(t, true)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	charlie, charlieToken := account(t, services, "charlie")
	owner := dial(t, server, aliceToken, "owner")
	receiver := dial(t, server, bobToken, "receiver")
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "POST", "/api/v1/conversations", model.CreateInput{Kind: "group", Name: "solo"}, 201), &created); err != nil || created.ID == "" {
		t.Fatalf("create group: %+v %v", created, err)
	}
	await(t, owner, "conversation.created", "")
	path := "/api/v1/conversations/" + created.ID
	httpCall(t, server, aliceToken, "POST", path+"/messages", map[string]string{"client_msg_id": "solo-message", "content": "before invite"}, 201)
	httpCall(t, server, bobToken, "GET", path+"/sync", nil, 404)
	httpCall(t, server, "", "POST", path+"/members", model.AddMembersInput{MemberIDs: []string{bob.ID}}, 401)
	httpCall(t, server, charlieToken, "POST", path+"/members", model.AddMembersInput{MemberIDs: []string{bob.ID}}, 404)
	httpCall(t, server, aliceToken, "POST", path+"/members", map[string]any{"member_ids": []string{bob.ID}, "unknown": true}, 400)
	httpCall(t, server, aliceToken, "POST", path+"/members", model.AddMembersInput{MemberIDs: []string{bob.ID}}, 200)
	for _, client := range []*peer{owner, receiver} {
		event := await(t, client, "conversation.created", "")
		var updated struct {
			ID string `json:"id"`
		}
		if err := json.Unmarshal(event.Data, &updated); err != nil || updated.ID != created.ID {
			t.Fatalf("membership refresh event: %+v %v", updated, err)
		}
	}
	var page model.SyncPage
	if err := json.Unmarshal(httpCall(t, server, bobToken, "GET", path+"/sync?after_seq=0", nil, 200), &page); err != nil || len(page.Messages) != 1 || page.Messages[0].Content != "before invite" {
		t.Fatalf("invited member history: %+v %v", page, err)
	}
	httpCall(t, server, aliceToken, "POST", path+"/members", model.AddMembersInput{MemberIDs: []string{bob.ID}}, 200)
	httpCall(t, server, aliceToken, "POST", path+"/messages", map[string]string{"client_msg_id": "invited-message", "content": "after invite"}, 201)
	var pushed model.Message
	if err := json.Unmarshal(await(t, receiver, "message.created", "").Data, &pushed); err != nil || pushed.Seq != 2 || pushed.Content != "after invite" {
		t.Fatalf("invited member push: %+v %v", pushed, err)
	}
	httpCall(t, server, bobToken, "POST", path+"/members", model.AddMembersInput{MemberIDs: []string{charlie.ID}}, 200)
	var groups []model.Conversation
	if err := json.Unmarshal(httpCall(t, server, charlieToken, "GET", "/api/v1/conversations", nil, 200), &groups); err != nil || len(groups) != 1 || len(groups[0].Members) != 3 {
		t.Fatalf("offline invited member discovery: %+v %v", groups, err)
	}
	if err := json.Unmarshal(httpCall(t, server, charlieToken, "GET", path+"/sync?after_seq=0", nil, 200), &page); err != nil || len(page.Messages) != 2 {
		t.Fatalf("offline invited member history: %+v %v", page, err)
	}
	_ = alice
}

func TestConcurrentGroupInvitationsRespectCapacityAndRetries(t *testing.T) {
	services := fixture(t, false)
	ctx := t.Context()
	owner, _ := account(t, services, "owner")
	if _, err := services.Store.DB.Exec(ctx, `INSERT INTO users(id,email,name,password_hash) SELECT 'invite-'||member,'invite-'||member||'@example.test','Member '||member,'unused' FROM generate_series(1,1000) AS member`); err != nil {
		t.Fatal(err)
	}
	memberIDs := make([]string, 998)
	for index := range memberIDs {
		memberIDs[index] = fmt.Sprintf("invite-%d", index+1)
	}
	group, err := services.Chat.Create(ctx, owner.ID, model.CreateInput{Kind: "group", Name: "almost full", MemberIDs: memberIDs})
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		memberID string
		err      error
	}
	results := make(chan result, 2)
	start := make(chan struct{})
	for _, memberID := range []string{"invite-999", "invite-1000"} {
		go func() {
			<-start
			results <- result{memberID: memberID, err: services.Chat.AddMembers(ctx, owner.ID, group, model.AddMembersInput{MemberIDs: []string{memberID}})}
		}()
	}
	close(start)
	var winner string
	var successes int
	for range 2 {
		outcome := <-results
		if outcome.err == nil {
			successes++
			winner = outcome.memberID
		} else {
			status(t, outcome.err, 400)
		}
	}
	if successes != 1 {
		t.Fatalf("expected one available slot, got %d successful invitations", successes)
	}
	if err = services.Chat.AddMembers(ctx, owner.ID, group, model.AddMembersInput{MemberIDs: []string{winner}}); err != nil {
		t.Fatalf("retry in full group: %v", err)
	}
	var total int
	if err = services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", group).Scan(&total); err != nil || total != 1000 {
		t.Fatalf("capacity exceeded: %d %v", total, err)
	}
	retryGroup, err := services.Chat.Create(ctx, owner.ID, model.CreateInput{Kind: "group", Name: "retry"})
	if err != nil {
		t.Fatal(err)
	}
	var wait sync.WaitGroup
	failures := make(chan error, 8)
	for range 8 {
		wait.Go(func() {
			failures <- services.Chat.AddMembers(ctx, owner.ID, retryGroup, model.AddMembersInput{MemberIDs: []string{"invite-1"}})
		})
	}
	wait.Wait()
	close(failures)
	for err := range failures {
		if err != nil {
			t.Fatal(err)
		}
	}
	if err = services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", retryGroup).Scan(&total); err != nil || total != 2 {
		t.Fatalf("duplicate members: %d %v", total, err)
	}
	if err = services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM message_outbox WHERE conversation_id=$1", retryGroup).Scan(&total); err != nil || total != 2 {
		t.Fatalf("retry emitted additional membership events: %d %v", total, err)
	}
}
