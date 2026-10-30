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
			if err := services.Store.DB.QueryRow(t.Context(), "SELECT recalled_at IS NOT NULL FROM messages WHERE id=$1", message.ID).Scan(&recalled); err != nil {
				t.Fatal(err)
			}
			if recalled != (tc.want == 200) {
				t.Fatalf("recalled=%v after status %d", recalled, tc.want)
			}
			if tc.want == 200 {
				var result model.Message
				if err := json.Unmarshal(raw, &result); err != nil {
					t.Fatal(err)
				}
				if result.ID != message.ID || result.Seq != message.Seq || result.RecalledAt == nil || result.Content != "" {
					t.Fatalf("invalid recalled response: %+v", result)
				}
			}
		})
	}
}
