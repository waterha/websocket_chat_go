package api

import (
	"encoding/json"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestRecallRoleAndTimeWindow(t *testing.T) {
	services, server, owner, ownerToken, member, memberToken, admin, adminToken, group := replyFixture(t)
	if err := services.Chat.SetRole(t.Context(), owner.ID, group, admin.ID, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	direct, err := services.Chat.Create(t.Context(), owner.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{admin.ID}})
	if err != nil {
		t.Fatal(err)
	}
	users := map[string]struct{ id, token string }{
		"owner": {owner.ID, ownerToken}, "admin": {admin.ID, adminToken}, "member": {member.ID, memberToken},
	}
	cases := []struct {
		name, conversation, sender, caller string
		expired                            bool
		want                               int
	}{
		{"owner_recalls_expired_member", group, "member", "owner", true, 200},
		{"admin_recalls_expired_member", group, "member", "admin", true, 200},
		{"owner_recalls_expired_admin", group, "admin", "owner", true, 200},
		{"admin_recalls_expired_owner", group, "owner", "admin", true, 200},
		{"owner_recalls_expired_self", group, "owner", "owner", true, 200},
		{"admin_recalls_expired_self", group, "admin", "admin", true, 200},
		{"member_cannot_recall_expired_self", group, "member", "member", true, 403},
		{"member_can_recall_recent_self", group, "member", "member", false, 200},
		{"member_cannot_recall_expired_owner", group, "owner", "member", true, 403},
		{"member_cannot_recall_recent_admin", group, "admin", "member", false, 403},
		{"direct_owner_cannot_recall_expired_self", direct, "owner", "owner", true, 403},
		{"direct_admin_cannot_recall_expired_self", direct, "admin", "admin", true, 403},
		{"direct_sender_can_recall_recent_self", direct, "owner", "owner", false, 200},
		{"direct_cannot_recall_recent_other", direct, "admin", "owner", false, 403},
		{"direct_cannot_recall_expired_other", direct, "admin", "owner", true, 403},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			message := sendMessage(t, server, users[tc.sender].token, tc.conversation, map[string]any{"client_msg_id": tc.name, "content": "recall permission target"}, 201)
			if tc.expired {
				if _, err := services.Store.DB.Exec(t.Context(), "UPDATE messages SET created_at=now()-interval '24 hours' WHERE id=$1", message.ID); err != nil {
					t.Fatal(err)
				}
			}
			raw := httpCall(t, server, users[tc.caller].token, "POST", "/api/v1/conversations/"+tc.conversation+"/messages/"+itoa(message.Seq)+"/recall", nil, tc.want)
			var recalled bool
			var actor string
			if err := services.Store.DB.QueryRow(t.Context(), "SELECT recalled_at IS NOT NULL,COALESCE(recalled_by,'') FROM messages WHERE id=$1", message.ID).Scan(&recalled, &actor); err != nil {
				t.Fatal(err)
			}
			wantActor := ""
			if tc.want == 200 {
				wantActor = users[tc.caller].id
			}
			if actor != wantActor {
				t.Fatalf("persisted recall actor = %q, want %q", actor, wantActor)
			}
			if recalled != (tc.want == 200) {
				t.Fatalf("recalled=%v after status %d", recalled, tc.want)
			}
			if tc.want == 200 {
				var result model.Message
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				if result.ID != message.ID || result.Seq != message.Seq || result.RecalledAt == nil || result.Content != "" || result.RecalledBy != users[tc.caller].id || result.RecalledByName != map[string]string{"owner": owner.Name, "admin": admin.Name, "member": member.Name}[tc.caller] {
					t.Fatalf("invalid recalled response: %+v", result)
				}
			}
		})
	}
}

func TestRecallActorSurvivesRetrySyncAndEvents(t *testing.T) {
	services, server, owner, ownerToken, member, memberToken, admin, adminToken, group := replyFixture(t)
	if err := services.Chat.SetRole(t.Context(), owner.ID, group, admin.ID, model.RoleAdmin); err != nil {
		t.Fatal(err)
	}
	message := sendMessage(t, server, memberToken, group, map[string]any{"client_msg_id": "recall-actor-001", "content": "withdraw this"}, 201)
	path := "/api/v1/conversations/" + group
	var first model.Message
	if err := json.Unmarshal(httpCall(t, server, adminToken, "POST", path+"/messages/"+itoa(message.Seq)+"/recall", nil, 200), &first); err != nil {
		t.Fatal(err)
	}
	check := func(message model.Message) {
		t.Helper()
		if message.RecalledBy != admin.ID || message.RecalledByName != admin.Name || message.SenderID != member.ID || message.RecalledAt == nil || !message.RecalledAt.Equal(*first.RecalledAt) || message.Content != "" {
			t.Fatalf("recall actor/state changed: %+v", message)
		}
	}
	check(first)
	// Another moderator retry must not replace the original actor or timestamp.
	var repeated model.Message
	if err := json.Unmarshal(httpCall(t, server, ownerToken, "POST", path+"/messages/"+itoa(message.Seq)+"/recall", nil, 200), &repeated); err != nil {
		t.Fatal(err)
	}
	check(repeated)
	var actor string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT recalled_by FROM messages WHERE id=$1", message.ID).Scan(&actor); err != nil || actor != admin.ID {
		t.Fatalf("persisted actor = %q: %v", actor, err)
	}
	// Persisted realtime payload uses exactly the same fields and emits once.
	var count int
	var payload []byte
	if err := services.Store.DB.QueryRow(t.Context(), `SELECT count(*),min(payload::text)::jsonb FROM message_outbox WHERE payload->>'event'='message.recalled' AND payload->'data'->>'id'=$1`, message.ID).Scan(&count, &payload); err != nil || count != 1 {
		t.Fatalf("recall outbox count = %d: %v", count, err)
	}
	var event struct {
		Data model.Message `json:"data"`
	}
	if err := json.Unmarshal(payload, &event); err != nil {
		t.Fatal(err)
	}
	check(event.Data)
	// A departed moderator's name still resolves independently of membership.
	if _, err := services.Store.DB.Exec(t.Context(), "DELETE FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", group, admin.ID); err != nil {
		t.Fatal(err)
	}
	for _, endpoint := range []string{"/sync?after_seq=0&limit=100", "/messages?before_seq=9999&limit=100"} {
		var messages []model.Message
		raw := httpCall(t, server, memberToken, "GET", path+endpoint, nil, 200)
		if endpoint == "/sync?after_seq=0&limit=100" {
			var page model.SyncPage
			if err := json.Unmarshal(raw, &page); err != nil {
				t.Fatal(err)
			}
			messages = page.Messages
		} else if err := json.Unmarshal(raw, &messages); err != nil {
			t.Fatal(err)
		}
		found := false
		for _, current := range messages {
			if current.ID == message.ID {
				found = true
				check(current)
			}
		}
		if !found {
			t.Fatalf("recalled message absent from %s", endpoint)
		}
	}
}
