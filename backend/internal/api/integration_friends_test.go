package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestFriendManagementHTTP(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	charlie, charlieToken := account(t, services, "charlie")

	var friends []model.User
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/friends", nil, 200), &friends); err != nil || len(friends) != 0 {
		t.Fatalf("initial friends: %+v %v", friends, err)
	}

	var rejected model.FriendRequest
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: bob.ID, Message: "想和你成为好友"}, 201), &rejected); err != nil {
		t.Fatal(err)
	}
	if rejected.Status != "pending" || rejected.Message != "想和你成为好友" || rejected.RequesterID != alice.ID || rejected.RecipientID != bob.ID {
		t.Fatalf("created request: %+v", rejected)
	}
	var duplicate model.FriendRequest
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: bob.ID}, 201), &duplicate); err != nil || duplicate.ID != rejected.ID {
		t.Fatalf("duplicate request: %+v %v", duplicate, err)
	}

	var bobRequests model.FriendRequestList
	if err := json.Unmarshal(httpCall(t, server, bobToken, "GET", "/api/v1/friends/requests", nil, 200), &bobRequests); err != nil || len(bobRequests.Incoming) != 1 {
		t.Fatalf("bob requests: %+v %v", bobRequests, err)
	}
	if err := json.Unmarshal(httpCall(t, server, bobToken, "POST", "/api/v1/friends/requests/"+rejected.ID+"/reject", nil, 200), &rejected); err != nil || rejected.Status != "rejected" {
		t.Fatalf("rejected request: %+v %v", rejected, err)
	}

	var aliceRequests model.FriendRequestList
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/friends/requests", nil, 200), &aliceRequests); err != nil || len(aliceRequests.Outgoing) != 1 || aliceRequests.Outgoing[0].Status != "rejected" {
		t.Fatalf("alice rejected request: %+v %v", aliceRequests, err)
	}

	var accepted model.FriendRequest
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: charlie.ID}, 201), &accepted); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(httpCall(t, server, charlieToken, "POST", "/api/v1/friends/requests/"+accepted.ID+"/accept", nil, 200), &accepted); err != nil || accepted.Status != "accepted" {
		t.Fatalf("accepted request: %+v %v", accepted, err)
	}

	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/friends", nil, 200), &friends); err != nil || len(friends) != 1 || friends[0].ID != charlie.ID {
		t.Fatalf("alice friends: %+v %v", friends, err)
	}
	if err := json.Unmarshal(httpCall(t, server, charlieToken, "GET", "/api/v1/friends", nil, 200), &friends); err != nil || len(friends) != 1 || friends[0].ID != alice.ID {
		t.Fatalf("charlie friends: %+v %v", friends, err)
	}
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/users", nil, 200), &friends); err != nil || len(friends) != 2 || !containsUser(friends, bob.ID) {
		t.Fatalf("user directory: %+v %v", friends, err)
	}

	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: alice.ID}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: "missing"}, 404)
}

func containsUser(users []model.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}
