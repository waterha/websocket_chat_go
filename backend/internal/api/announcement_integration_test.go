package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

// TestAnnouncementCreateEditDelete covers the collection semantics: a member
// reads every notice, while only owners and admins create, edit and delete them.
func TestAnnouncementCreateEditDelete(t *testing.T) {
	services, server, _, aliceToken, bob, bobToken, _, carolToken, group := groupFixture(t)

	// Bob is promoted so the test covers an admin as well as the owner.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/roles", map[string]string{"user_id": bob.ID, "role": "admin"}, 200)

	// A plain member may read the (empty) list but not publish.
	httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/announcements", nil, 200)
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "我不想当管理员"}, 403)

	// The owner publishes two notices; both are kept rather than replacing one.
	first := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "周一全员同步"}, 201)
	var one model.Announcement
	if err := json.Unmarshal(first, &one); err != nil || one.Content != "周一全员同步" || one.AuthorID == "" || one.Edited {
		t.Fatalf("unexpected first announcement: %+v %v", one, err)
	}
	second := httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "周五下午茶"}, 201)
	var two model.Announcement
	if err := json.Unmarshal(second, &two); err != nil || two.Content != "周五下午茶" {
		t.Fatalf("unexpected second announcement: %+v %v", two, err)
	}
	if two.ID == one.ID {
		t.Fatal("each announcement must be its own record")
	}

	// The list returns both, newest first, to any member.
	raw := httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/announcements", nil, 200)
	var list []model.Announcement
	if err := json.Unmarshal(raw, &list); err != nil {
		t.Fatal(err)
	}
	if len(list) != 2 {
		t.Fatalf("expected two announcements, got %d", len(list))
	}
	if list[0].ID != two.ID || list[1].ID != one.ID {
		t.Fatalf("announcements are not newest first: %+v", list)
	}

	// An edit changes the text and marks the record edited, keeping the author.
	edited := httpCall(t, server, bobToken, "PATCH", "/api/v1/conversations/"+group+"/announcements/"+one.ID, map[string]string{"content": "周一改为全员复盘"}, 200)
	var updated model.Announcement
	if err := json.Unmarshal(edited, &updated); err != nil {
		t.Fatal(err)
	}
	if updated.Content != "周一改为全员复盘" || !updated.Edited || updated.AuthorID != one.AuthorID {
		t.Fatalf("unexpected edit result: %+v", updated)
	}

	// A plain member cannot edit or delete.
	httpCall(t, server, carolToken, "PATCH", "/api/v1/conversations/"+group+"/announcements/"+one.ID, map[string]string{"content": "被篡改"}, 403)
	httpCall(t, server, carolToken, "DELETE", "/api/v1/conversations/"+group+"/announcements/"+one.ID, nil, 403)

	// Deleting one notice leaves the other intact.
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/conversations/"+group+"/announcements/"+one.ID, nil, 200)
	after := httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/announcements", nil, 200)
	var remaining []model.Announcement
	if err := json.Unmarshal(after, &remaining); err != nil {
		t.Fatal(err)
	}
	if len(remaining) != 1 || remaining[0].ID != two.ID {
		t.Fatalf("deleting one notice removed the wrong records: %+v", remaining)
	}

	// The count travels with the conversation so the panel can show it.
	conversations := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations", nil, 200)
	var items []model.Conversation
	if err := json.Unmarshal(conversations, &items); err != nil {
		t.Fatal(err)
	}
	if len(items) != 1 || items[0].AnnouncementCount != 1 {
		t.Fatalf("the conversation should report one announcement: %+v", items)
	}
	_ = services
}

// Validation and lookup errors must be explicit rather than silently ignored.
func TestAnnouncementValidationAndLookup(t *testing.T) {
	services, server, _, aliceToken, _, _, _, _, group := groupFixture(t)

	// Empty, whitespace-only and oversized text are refused.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": ""}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "   "}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": string(make([]rune, 2001))}, 400)

	created := httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "边界测试"}, 201)
	var one model.Announcement
	if err := json.Unmarshal(created, &one); err != nil {
		t.Fatal(err)
	}

	// An unknown id is not found for both edit and delete.
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+group+"/announcements/does-not-exist", map[string]string{"content": "x"}, 404)
	httpCall(t, server, aliceToken, "DELETE", "/api/v1/conversations/"+group+"/announcements/does-not-exist", nil, 404)
	// An announcement id from another conversation is not found either, so a
	// notice cannot be edited across conversations.
	other, err := groupFixtureCreate(t, aliceToken, server)
	if err != nil {
		t.Fatal(err)
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/conversations/"+other+"/announcements/"+one.ID, map[string]string{"content": "跨会话"}, 404)

	// A non-member cannot read the list at all. A member of the group can, which
	// the earlier call already proved.
	_, outsiderToken := account(t, services, "announcement-outsider")
	httpCall(t, server, outsiderToken, "GET", "/api/v1/conversations/"+group+"/announcements", nil, 404)
}

// groupFixtureCreate makes a second group owned by the token's account.
func groupFixtureCreate(t *testing.T, token string, server *httptest.Server) (string, error) {
	t.Helper()
	raw := httpCall(t, server, token, "POST", "/api/v1/conversations", model.CreateInput{Kind: "group", Name: "another-group"}, 201)
	var created struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(raw, &created); err != nil {
		return "", err
	}
	return created.ID, nil
}
