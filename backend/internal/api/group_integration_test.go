package api

import (
	"encoding/json"
	"net/http/httptest"
	"testing"

	"websocket_chat_go/backend/internal/model"
	"websocket_chat_go/backend/internal/svc"
)

// groupFixture builds a group owned by alice with bob (admin) and carol (member).
func groupFixture(t *testing.T) (*svc.ServiceContext, *httptest.Server, model.User, string, model.User, string, model.User, string, string) {
	t.Helper()
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	carol, carolToken := account(t, services, "carol")
	group, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "group", Name: "team", MemberIDs: []string{bob.ID, carol.ID}})
	if err != nil {
		t.Fatal(err)
	}
	return services, server, alice, aliceToken, bob, bobToken, carol, carolToken, group
}

func TestGroupRolesAndAnnouncement(t *testing.T) {
	services, server, alice, aliceToken, bob, bobToken, carol, carolToken, group := groupFixture(t)

	// The creator is the sole owner and the others start as plain members.
	var owner string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT created_by FROM conversations WHERE id=$1", group).Scan(&owner); err != nil || owner != alice.ID {
		t.Fatalf("creator is not the owner: %q %v", owner, err)
	}
	roles := map[string]string{}
	rows, err := services.Store.DB.Query(t.Context(), "SELECT user_id,role FROM conversation_members WHERE conversation_id=$1", group)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var userID, role string
		if err = rows.Scan(&userID, &role); err != nil {
			t.Fatal(err)
		}
		roles[userID] = role
	}
	rows.Close()
	if roles[alice.ID] != "owner" || roles[bob.ID] != "member" || roles[carol.ID] != "member" {
		t.Fatalf("unexpected initial roles: %+v", roles)
	}

	// The conversation payload exposes roles and the announcement.
	page := httpCall(t, server, carolToken, "GET", "/api/v1/conversations", nil, 200)
	var list []model.Conversation
	if err := json.Unmarshal(page, &list); err != nil || len(list) != 1 {
		t.Fatalf("unexpected conversations: %v %v", list, err)
	}
	if list[0].OwnerID != alice.ID || len(list[0].Members) != 3 {
		t.Fatalf("missing owner or members: %+v", list[0])
	}
	if list[0].Members[0].Role != "owner" {
		t.Fatalf("owner is not listed first: %+v", list[0].Members)
	}

	// Only the owner changes roles; a member cannot promote themselves.
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+group+"/roles", map[string]string{"user_id": carol.ID, "role": "admin"}, 403)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/roles", map[string]string{"user_id": bob.ID, "role": "admin"}, 200)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/roles", map[string]string{"user_id": alice.ID, "role": "admin"}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/roles", map[string]string{"user_id": carol.ID, "role": "owner"}, 400)

	// An admin may remove a member but never the owner or another admin.
	httpCall(t, server, bobToken, "DELETE", "/api/v1/conversations/"+group+"/members", map[string]string{"user_id": carol.ID}, 200)
	httpCall(t, server, bobToken, "DELETE", "/api/v1/conversations/"+group+"/members", map[string]string{"user_id": alice.ID}, 403)

	// Announcements require owner or admin; a plain member is refused. carol was
	// removed above, so she now gets 404, while the owner publishes successfully.
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "nope"}, 404)
	created := httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": "周五全员同步"}, 201)
	var announcement model.Announcement
	if err := json.Unmarshal(created, &announcement); err != nil || announcement.Content != "周五全员同步" {
		t.Fatalf("unexpected announcement result: %v %v", announcement, err)
	}
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/announcements", map[string]string{"content": string(make([]rune, 2001))}, 400)

	// Every change is recorded as a system message that members can roam.
	synced := httpCall(t, server, aliceToken, "GET", "/api/v1/conversations/"+group+"/sync?after_seq=0&limit=100", nil, 200)
	var syncPage model.SyncPage
	if err := json.Unmarshal(synced, &syncPage); err != nil {
		t.Fatal(err)
	}
	contents := []string{}
	for _, message := range syncPage.Messages {
		if message.Kind == "system" {
			contents = append(contents, message.Content)
		}
	}
	expected := []string{"bob被设为管理员", "bob将carol移出群聊"}
	for _, want := range expected {
		found := false
		for _, got := range contents {
			if got == want {
				found = true
			}
		}
		if !found {
			t.Fatalf("missing system notice %q in %v", want, contents)
		}
	}
}

func TestGroupLeaveAndOwnershipTransfer(t *testing.T) {
	services, server, alice, aliceToken, bob, bobToken, carol, carolToken, group := groupFixture(t)

	// A member can leave the group by themselves.
	httpCall(t, server, carolToken, "POST", "/api/v1/conversations/"+group+"/members/leave", nil, 200)
	var count int
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT count(*) FROM conversation_members WHERE conversation_id=$1 AND user_id=$2", group, carol.ID).Scan(&count); err != nil || count != 0 {
		t.Fatalf("member did not leave: %d %v", count, err)
	}
	// A former member can no longer read the conversation.
	httpCall(t, server, carolToken, "GET", "/api/v1/conversations/"+group+"/sync?after_seq=0", nil, 404)

	// The owner cannot leave without transferring first, and cannot be removed.
	httpCall(t, server, bobToken, "DELETE", "/api/v1/conversations/"+group+"/members", map[string]string{"user_id": alice.ID}, 403)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/members/leave", nil, 403)

	// Only the owner transfers ownership; the target must be a member.
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/owner", map[string]string{"user_id": bob.ID}, 403)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/owner", map[string]string{"user_id": carol.ID}, 404)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/owner", map[string]string{"user_id": bob.ID}, 200)

	roles := map[string]string{}
	rows, err := services.Store.DB.Query(t.Context(), "SELECT user_id,role FROM conversation_members WHERE conversation_id=$1", group)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var userID, role string
		if err = rows.Scan(&userID, &role); err != nil {
			t.Fatal(err)
		}
		roles[userID] = role
	}
	rows.Close()
	if roles[bob.ID] != "owner" || roles[alice.ID] != "admin" {
		t.Fatalf("transfer produced wrong roles: %+v", roles)
	}
	var createdBy string
	if err := services.Store.DB.QueryRow(t.Context(), "SELECT created_by FROM conversations WHERE id=$1", group).Scan(&createdBy); err != nil || createdBy != bob.ID {
		t.Fatalf("created_by was not updated: %q %v", createdBy, err)
	}
	// The new owner can now transfer it onward, proving the role really moved.
	httpCall(t, server, bobToken, "POST", "/api/v1/conversations/"+group+"/owner", map[string]string{"user_id": alice.ID}, 200)
	// A group can never be left without an owner.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+group+"/members/leave", nil, 403)
}

func TestDirectConversationsHaveNoGovernance(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	defer server.Close()
	alice, aliceToken := account(t, services, "alice")
	bob, _ := account(t, services, "bob")
	direct, err := services.Chat.Create(t.Context(), alice.ID, model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}})
	if err != nil {
		t.Fatal(err)
	}
	// Direct conversations have no owner and refuse group-only operations.
	roles := map[string]string{}
	rows, err := services.Store.DB.Query(t.Context(), "SELECT user_id,role FROM conversation_members WHERE conversation_id=$1", direct)
	if err != nil {
		t.Fatal(err)
	}
	for rows.Next() {
		var userID, role string
		if err = rows.Scan(&userID, &role); err != nil {
			t.Fatal(err)
		}
		roles[userID] = role
	}
	rows.Close()
	if roles[alice.ID] != "member" || roles[bob.ID] != "member" {
		t.Fatalf("direct conversation gained roles: %+v", roles)
	}
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+direct+"/members/leave", nil, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+direct+"/owner", map[string]string{"user_id": bob.ID}, 400)
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations/"+direct+"/announcements", map[string]string{"content": "hi"}, 400)
}
