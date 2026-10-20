package api

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/config"
	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

// TestCrossInstanceDelivery is the regression test for the audit finding that a
// drained outbox event reached whichever instance won the drain lock and no
// other. Two independent instances (separate pools, Hubs and workers) share one
// schema; the same user holds a connection on each; every message must now reach
// both connections exactly once.
func TestCrossInstanceDelivery(t *testing.T) {
	databaseURL, redisAddr := os.Getenv("IM_TEST_DATABASE_URL"), os.Getenv("IM_TEST_REDIS_ADDR")
	if databaseURL == "" || redisAddr == "" {
		t.Skip("set IM_TEST_DATABASE_URL and IM_TEST_REDIS_ADDR")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	schema := "im_fanout_" + model.NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		drop, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if _, err := admin.Exec(drop, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Logf("fan-out schema cleanup: %v", err)
		}
	})

	newInstance := func(instanceID string) *svc.ServiceContext {
		pool, err := pgxpool.ParseConfig(databaseURL)
		if err != nil {
			t.Fatal(err)
		}
		pool.ConnConfig.RuntimeParams["search_path"] = schema
		// A distinct instance identity is what the fan-out subscriber uses to
		// skip its own publications.
		t.Setenv("INSTANCE_ID", instanceID)
		settings := config.Config{Postgres: pool, RedisAddr: redisAddr, Origin: "http://im.test",
			MigrationsDir: filepath.Join("..", "..", "..", "database", "migrations"),
			MediaDir:      filepath.Join(t.TempDir(), "media"),
			Heartbeat:     time.Second, HeartbeatTimeout: time.Second}
		instance, err := svc.New(ctx, settings)
		if err != nil {
			t.Fatal(err)
		}
		if instance.Broadcast.InstanceID != instanceID {
			t.Fatalf("instance identity was not applied: %q", instance.Broadcast.InstanceID)
		}
		return instance
	}
	first, second := newInstance("instance-one"), newInstance("instance-two")
	workers, cancel := context.WithCancel(ctx)
	// Cancel before Close: Close waits for the worker goroutines.
	t.Cleanup(func() {
		cancel()
		first.Close()
		second.Close()
	})

	alice, _ := account(t, first, "fanout-alice")
	bob, _ := account(t, first, "fanout-bob")
	group, err := first.Chat.Create(ctx, alice.ID, model.CreateInput{Kind: "group", Name: "fanout", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	first.Start(workers)
	second.Start(workers)
	// Let both subscribers confirm before events are produced.
	time.Sleep(500 * time.Millisecond)

	// Bob holds one connection per instance.
	clientFirst, err := first.Hub.Register(ctx, bob.ID, "device-one", "session-one")
	if err != nil {
		t.Fatal(err)
	}
	clientSecond, err := second.Hub.Register(ctx, bob.ID, "device-two", "session-two")
	if err != nil {
		t.Fatal(err)
	}

	onFirst := map[string]int{}
	onSecond := map[string]int{}
	collect := func(target map[string]int, channel chan model.Event) {
		for {
			select {
			case event := <-channel:
				if event.Event != "message.created" {
					continue
				}
				payload, err := json.Marshal(event.Data)
				if err != nil {
					continue
				}
				var message model.Message
				if json.Unmarshal(payload, &message) != nil || message.ID == "" {
					continue
				}
				target[message.ID]++
			default:
				return
			}
		}
	}

	const rounds = 10
	for round := 0; round < rounds; round++ {
		if _, _, err := first.Chat.Send(ctx, alice.ID, model.SendInput{
			ConversationID: group,
			ClientMsgID:    "fanout-round-" + string(rune('a'+round)) + "-0001",
			Content:        "fanout",
		}); err != nil {
			t.Fatal(err)
		}
		// Allow the drain lock, the publish and the subscriber to complete.
		time.Sleep(400 * time.Millisecond)
		collect(onFirst, clientFirst.Outbound)
		collect(onSecond, clientSecond.Outbound)
	}
	// One more grace period for late deliveries.
	time.Sleep(500 * time.Millisecond)
	collect(onFirst, clientFirst.Outbound)
	collect(onSecond, clientSecond.Outbound)

	var missingFirst, missingSecond, duplicated int
	for round := 0; round < rounds; round++ {
		id := sentID(t, first, alice.ID, "fanout-round-"+string(rune('a'+round))+"-0001")
		gotFirst, gotSecond := onFirst[id], onSecond[id]
		if gotFirst == 0 {
			missingFirst++
		}
		if gotSecond == 0 {
			missingSecond++
		}
		if gotFirst > 1 || gotSecond > 1 {
			duplicated++
		}
	}
	t.Logf("CROSS-INSTANCE: missingOnFirst=%d missingOnSecond=%d duplicated=%d", missingFirst, missingSecond, duplicated)
	if missingFirst != 0 || missingSecond != 0 {
		t.Fatalf("every message must reach both instances: first=%d second=%d", missingFirst, missingSecond)
	}
	if duplicated != 0 {
		t.Fatalf("no message may be delivered twice to one instance: %d", duplicated)
	}
}

// sentID resolves the message id for a client message id, so the assertion does
// not depend on capture ordering.
func sentID(t *testing.T, services *svc.ServiceContext, senderID, clientMsgID string) string {
	t.Helper()
	var id string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT id FROM messages WHERE sender_id=$1 AND client_msg_id=$2", senderID, clientMsgID).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}
