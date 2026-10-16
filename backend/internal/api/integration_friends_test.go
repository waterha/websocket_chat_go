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
	charlie, _ := account(t, services, "charlie")

	var friends []model.User
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/friends", nil, 200), &friends); err != nil || len(friends) != 0 {
		t.Fatalf("initial friends: %+v %v", friends, err)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: bob.ID}, 200)
	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: bob.ID}, 200)
	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: alice.ID}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/friends", model.AddFriendInput{UserID: "missing"}, 404)

	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/friends", nil, 200), &friends); err != nil || len(friends) != 1 || friends[0].ID != bob.ID {
		t.Fatalf("alice friends: %+v %v", friends, err)
	}
	if err := json.Unmarshal(httpCall(t, server, bobToken, "GET", "/api/v1/friends", nil, 200), &friends); err != nil || len(friends) != 1 || friends[0].ID != alice.ID {
		t.Fatalf("bob friends: %+v %v", friends, err)
	}
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/users", nil, 200), &friends); err != nil || len(friends) != 2 || !containsUser(friends, charlie.ID) {
		t.Fatalf("user directory: %+v %v", friends, err)
	}
}

func containsUser(users []model.User, id string) bool {
	for _, user := range users {
		if user.ID == id {
			return true
		}
	}
	return false
}
