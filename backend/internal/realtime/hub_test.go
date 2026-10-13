package realtime

import (
	"context"
	"fmt"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestHubRoutingReplacementAndBackpressure(t *testing.T) {
	hub := NewHub(2)
	defer hub.Close()
	register := func(user, device string) *Client {
		t.Helper()
		client, err := hub.Register(context.Background(), user, device, user)
		if err != nil {
			t.Fatal(err)
		}
		return client
	}
	first := register("alice", "phone")
	second := register("alice", "desktop")
	outsider := register("outsider", "phone")
	hub.Publish([]string{"alice"}, model.Event{Event: "message.created"})
	for _, client := range []*Client{first, second} {
		select {
		case event := <-client.Outbound:
			if event.Event != "message.created" {
				t.Fatal(event)
			}
		default:
			t.Fatal("missing device delivery")
		}
	}
	if len(outsider.Outbound) != 0 {
		t.Fatal("routed to non-member")
	}
	replacement := register("alice", "phone")
	if first.Context.Err() == nil {
		t.Fatal("old device still active")
	}
	hub.Unregister(first)
	hub.Publish([]string{"alice"}, model.Event{Event: "replacement"})
	if len(replacement.Outbound) != 1 {
		t.Fatal("old unregister removed new connection")
	}
	if !replacement.Reply(model.Event{}) || replacement.Reply(model.Event{}) || replacement.Context.Err() == nil {
		t.Fatal("slow consumer was not canceled")
	}
	hub.DisconnectSession("outsider")
	if outsider.Context.Err() == nil {
		t.Fatal("logout did not cancel session")
	}
	hub.Close()
	if _, err := hub.Register(context.Background(), "bob", "phone", "bob"); err == nil {
		t.Fatal("registered after shutdown")
	}
}

func TestHubDeviceLimitAndConcurrentPublish(t *testing.T) {
	hub := NewHub(1024)
	defer hub.Close()
	for index := 0; index < 8; index++ {
		if _, err := hub.Register(context.Background(), "user", fmt.Sprint(index), "session"); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := hub.Register(context.Background(), "user", "ninth", "session"); err == nil {
		t.Fatal("device limit bypassed")
	}
	t.Run("parallel", func(t *testing.T) {
		for index := 0; index < 20; index++ {
			t.Run(fmt.Sprint(index), func(t *testing.T) { t.Parallel(); hub.Publish([]string{"user"}, model.Event{Event: "test"}) })
		}
	})
}
