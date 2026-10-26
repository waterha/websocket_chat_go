package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestGroupDiscoveryAndJoinApplications(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	ctx := t.Context()
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, carolToken := account(t, services, "carol")

	openGroup, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "open-room"})
	if err != nil {
		t.Fatal(err)
	}
	gatedGroup, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "gated-room"})
	if err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+gatedGroup+"/settings", map[string]any{"join_approval": true}, 200)
	direct, err := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{carol.ID}})
	if err != nil {
		t.Fatal(err)
	}

	raw := httpCall(t, server, bobToken, "GET", "/api/v1/groups/discover?q="+gatedGroup, nil, 200)
	var groups []model.DiscoverableGroup
	if err = json.Unmarshal(raw, &groups); err != nil || len(groups) != 1 {
		t.Fatalf("discovery by ID failed: %+v %v", groups, err)
	}
	if groups[0].ID != gatedGroup || !groups[0].JoinApproval || groups[0].MemberCount != 1 || groups[0].RequestStatus != "" {
		t.Fatalf("unexpected discovered gated group: %+v", groups[0])
	}
	raw = httpCall(t, server, bobToken, "GET", "/api/v1/groups/discover?q=open-room", nil, 200)
	if err = json.Unmarshal(raw, &groups); err != nil || len(groups) != 1 || groups[0].JoinApproval {
		t.Fatalf("discovery by name failed: %+v %v", groups, err)
	}
	if err = json.Unmarshal(httpCall(t, server, bobToken, "GET", "/api/v1/groups/discover?q="+direct, nil, 200), &groups); err != nil || len(groups) != 0 {
		t.Fatalf("direct conversation appeared as a group: %+v %v", groups, err)
	}

	var joined map[string]any
	err = json.Unmarshal(httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+openGroup+"/join-requests", map[string]string{"message": ""}, 200), &joined)
	if err != nil || joined["pending"] != false || joined["id"] != openGroup {
		t.Fatalf("open group should add immediately: %+v %v", joined, err)
	}
	if _, err = services.Chat.RequestToJoin(ctx, bob.ID, openGroup, ""); err == nil {
		t.Fatal("duplicate direct join should not be treated as success")
	}

	for i := 0; i < 2; i++ {
		err = json.Unmarshal(httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+gatedGroup+"/join-requests", map[string]string{"message": "Please let me in"}, 200), &joined)
		if err != nil || joined["pending"] != true || joined["id"] == "" {
			t.Fatalf("gated group should return an idempotent pending application: %+v %v", joined, err)
		}
	}
	raw = httpCall(t, server, bobToken, "GET", "/api/v1/groups/discover?q="+gatedGroup, nil, 200)
	if err = json.Unmarshal(raw, &groups); err != nil || groups[0].RequestStatus != "pending" || groups[0].RequestID != joined["id"] {
		t.Fatalf("pending application missing in discovery: %+v %v", groups, err)
	}
	raw = httpCall(t, server, bobToken, "GET", "/api/v1/groups/join-requests", nil, 200)
	var history []map[string]any
	if err = json.Unmarshal(raw, &history); err != nil || len(history) != 1 || history[0]["group_name"] != "gated-room" || history[0]["message"] != "Please let me in" || history[0]["status"] != "pending" {
		t.Fatalf("unexpected application history: %+v %v", history, err)
	}
	httpCall(t, server, carolToken, "DELETE", "/api/v1/groups/"+gatedGroup+"/join-requests", nil, 404)
	httpCall(t, server, bobToken, "DELETE", "/api/v1/groups/"+gatedGroup+"/join-requests", nil, 200)
	raw = httpCall(t, server, bobToken, "GET", "/api/v1/groups/join-requests", nil, 200)
	if err = json.Unmarshal(raw, &history); err != nil || len(history) != 0 {
		t.Fatalf("withdrawn application should be absent from history: %+v %v", history, err)
	}

	// An approval admits the applicant, retains its decision and protects history.
	httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+gatedGroup+"/join-requests", map[string]string{"message": "approve me"}, 200)
	var approvalRequests []model.JoinRequest
	if err = json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+gatedGroup+"/join-requests", nil, 200), &approvalRequests); err != nil || len(approvalRequests) != 1 {
		t.Fatalf("approval requests: %+v %v", approvalRequests, err)
	}
	httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+gatedGroup+"/join-requests", nil, 404)
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+gatedGroup+"/join-requests/"+approvalRequests[0].ID+"/accept", nil, 404)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+gatedGroup+"/join-requests/"+approvalRequests[0].ID+"/accept", nil, 200)
	if err = json.Unmarshal(httpCall(t, server, bobToken, "GET", "/api/v1/groups/join-requests", nil, 200), &history); err != nil || len(history) != 1 || history[0]["status"] != "accepted" {
		t.Fatalf("accepted application history: %+v %v", history, err)
	}
	httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+gatedGroup+"/messages", nil, 200)
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+gatedGroup+"/members/leave", nil, 200)

	// Rejection remains visible and never grants membership.
	httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+gatedGroup+"/join-requests", map[string]string{"message": "reject me"}, 200)
	if err = json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+gatedGroup+"/join-requests", nil, 200), &approvalRequests); err != nil || len(approvalRequests) != 1 {
		t.Fatalf("rejection requests: %+v %v", approvalRequests, err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+gatedGroup+"/join-requests/"+approvalRequests[0].ID+"/reject", nil, 200)
	if err = json.Unmarshal(httpCall(t, server, bobToken, "GET", "/api/v1/groups/join-requests", nil, 200), &history); err != nil || len(history) != 2 || history[0]["status"] != "rejected" {
		t.Fatalf("rejected application history: %+v %v", history, err)
	}
	httpCall(t, server, bobToken, "GET", "/api/v1/conversations/"+gatedGroup+"/messages", nil, 404)

	// Capacity is checked while holding the group lock for direct and gated joins.
	fullGroup, createErr := services.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "full-room"})
	if createErr != nil {
		t.Fatal(createErr)
	}
	if _, err = services.Store.DB.Exec(ctx, `INSERT INTO users(id,email,name,password_hash)
		SELECT 'capacity-'||g::text,'capacity-'||g::text||'@test','capacity','unused' FROM generate_series(1,999) g`); err != nil {
		t.Fatal(err)
	}
	if _, err = services.Store.DB.Exec(ctx, `INSERT INTO conversation_members(conversation_id,user_id)
		SELECT $1,'capacity-'||g::text FROM generate_series(1,999) g`, fullGroup); err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+fullGroup+"/join-requests", map[string]string{"message": ""}, 400)
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+fullGroup+"/settings", map[string]any{"join_approval": true}, 200)
	httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+fullGroup+"/join-requests", map[string]string{"message": ""}, 400)

	// Concurrent retries share one pending request even across transactions.
	type joinResult struct {
		request model.JoinRequest
		err     error
	}
	results := make(chan joinResult, 8)
	for i := 0; i < 8; i++ {
		go func() {
			request, joinErr := services.Chat.RequestToJoin(ctx, bob.ID, gatedGroup, "retry")
			results <- joinResult{request, joinErr}
		}()
	}
	requestID := ""
	for i := 0; i < 8; i++ {
		result := <-results
		if result.err != nil {
			t.Fatal(result.err)
		}
		if requestID != "" && result.request.ID != requestID {
			t.Fatalf("concurrent retries created different requests: %s %s", requestID, result.request.ID)
		}
		requestID = result.request.ID
	}

	// Two callers racing for the last slot cannot push membership above 1000.
	if _, err = services.Store.DB.Exec(ctx, "DELETE FROM conversation_members WHERE conversation_id=$1 AND user_id='capacity-999'", fullGroup); err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+fullGroup+"/settings", map[string]any{"join_approval": false}, 200)
	for _, userID := range []string{bob.ID, carol.ID} {
		go func(id string) {
			request, joinErr := services.Chat.RequestToJoin(ctx, id, fullGroup, "")
			results <- joinResult{request, joinErr}
		}(userID)
	}
	succeeded := 0
	for i := 0; i < 2; i++ {
		result := <-results
		if result.err == nil {
			succeeded++
		} else {
			status(t, result.err, 400)
		}
	}
	var memberCount int
	if err = services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM conversation_members WHERE conversation_id=$1", fullGroup).Scan(&memberCount); err != nil || memberCount != 1000 || succeeded != 1 {
		t.Fatalf("capacity race: count=%d successes=%d err=%v", memberCount, succeeded, err)
	}

	// A pending request cannot be accepted after the group is dismissed.
	httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+gatedGroup+"/join-requests", map[string]string{"message": "retry"}, 200)
	raw = httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+gatedGroup+"/join-requests", nil, 200)
	var pending []model.JoinRequest
	if err = json.Unmarshal(raw, &pending); err != nil || len(pending) != 1 || !pending[0].IsApplication {
		t.Fatalf("application not visible to group admin: %+v %v", pending, err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+gatedGroup+"/dismiss", nil, 200)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+gatedGroup+"/join-requests/"+pending[0].ID+"/accept", nil, 409)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+gatedGroup+"/join-requests/"+pending[0].ID+"/reject", nil, 200)
	httpCall(t, server, bobToken, "POST", "/api/v1/groups/"+gatedGroup+"/join-requests", map[string]string{"message": "again"}, 409)
}
