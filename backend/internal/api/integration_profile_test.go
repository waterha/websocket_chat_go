package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestProfileUpdate(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, _ := account(t, services, "bob")

	var updated model.User
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "PATCH", "/api/v1/me", model.ProfileInput{Email: "alice-new@example.test", Name: "Alice New"}, 200), &updated); err != nil {
		t.Fatal(err)
	}
	if updated.ID != alice.ID || updated.Email != "alice-new@example.test" || updated.Name != "Alice New" {
		t.Fatalf("updated profile: %+v", updated)
	}

	var current model.User
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/me", nil, 200), &current); err != nil {
		t.Fatal(err)
	}
	if current.Email != updated.Email || current.Name != updated.Name {
		t.Fatalf("authenticated profile: %+v", current)
	}

	httpCall(t, server, aliceToken, "PATCH", "/api/v1/me", model.ProfileInput{Email: bob.Email, Name: "Alice New"}, 409)
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/me", model.ProfileInput{Email: "invalid", Name: ""}, 400)
}
