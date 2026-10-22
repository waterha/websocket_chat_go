package api

import (
	"encoding/json"
	"fmt"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestColleagueDirectory(t *testing.T) {
	services := fixture(t, false)
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	alice, aliceToken := account(t, services, "alice")
	bob, bobToken := account(t, services, "bob")
	_, carolToken := account(t, services, "carol")
	decodePage := func(path string) model.DirectoryPage {
		t.Helper()
		var page model.DirectoryPage
		if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", path, nil, 200), &page); err != nil {
			t.Fatal(err)
		}
		return page
	}
	httpCall(t, server, "", "GET", "/api/v1/directory", nil, 401)
	httpCall(t, server, "", "GET", "/api/v1/directory/me", nil, 401)
	httpCall(t, server, "", "PATCH", "/api/v1/directory/me", model.WorkProfile{}, 401)
	var blank model.WorkProfile
	if err := json.Unmarshal(httpCall(t, server, aliceToken, "GET", "/api/v1/directory/me", nil, 200), &blank); err != nil {
		t.Fatal(err)
	}
	if blank.Department != "" || blank.JobTitle != "" {
		t.Fatal(blank)
	}
	httpCall(t, server, bobToken, "PATCH", "/api/v1/directory/me", model.WorkProfile{Department: "  研发部  ", JobTitle: " Go 工程师 "}, 200)
	httpCall(t, server, carolToken, "PATCH", "/api/v1/directory/me", model.WorkProfile{Department: "运营部", JobTitle: "分析师"}, 200)
	page := decodePage("/api/v1/directory?q=" + url.QueryEscape("工程师"))
	if len(page.Entries) != 1 || page.Entries[0].ID != bob.ID || page.Entries[0].Department != "研发部" || page.Entries[0].JobTitle != "Go 工程师" {
		t.Fatalf("bad search: %+v", page)
	}
	for _, query := range []string{"BOB", "bob@example.test", "研发部"} {
		page = decodePage("/api/v1/directory?q=" + url.QueryEscape(query))
		if len(page.Entries) != 1 || page.Entries[0].ID != bob.ID {
			t.Fatalf("bad search %q: %+v", query, page)
		}
	}
	page = decodePage("/api/v1/directory?department=" + url.QueryEscape("研发部"))
	if len(page.Entries) != 1 || len(page.Departments) != 2 {
		t.Fatalf("bad department filter: %+v", page)
	}
	for _, query := range []string{"%", "_", "alice"} {
		if page := decodePage("/api/v1/directory?q=" + url.QueryEscape(query)); len(page.Entries) != 0 || page.Entries == nil {
			t.Fatalf("literal/self search %q: %+v", query, page)
		}
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/directory/me", map[string]string{"user_id": bob.ID, "department": "other"}, 400)
	for _, value := range []string{strings.Repeat("部", 101), "a\nb", "a\x00b"} {
		httpCall(t, server, aliceToken, "PATCH", "/api/v1/directory/me", model.WorkProfile{Department: value}, 400)
	}
	for _, query := range []string{"offset=-1", "offset=foo", "limit=0", "limit=101", "limit=", "offset=1000001", "q=" + strings.Repeat("a", 201)} {
		httpCall(t, server, aliceToken, "GET", "/api/v1/directory?"+query, nil, 400)
	}
	page = decodePage("/api/v1/directory?limit=1")
	next := decodePage(fmt.Sprintf("/api/v1/directory?limit=1&offset=%d", page.NextOffset))
	if !page.HasMore || page.NextOffset != 1 || next.HasMore || len(next.Entries) != 1 || page.Entries[0].ID == next.Entries[0].ID {
		t.Fatalf("pagination failed: %+v %+v", page, next)
	}
	// Only the caller's profile changes, and clearing it persists.
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/directory/me", model.WorkProfile{Department: "财务"}, 200)
	other, err := services.Auth.WorkProfile(t.Context(), bob.ID)
	if err != nil || other.Department != "研发部" {
		t.Fatalf("other profile changed: %+v %v", other, err)
	}
	httpCall(t, server, aliceToken, "PATCH", "/api/v1/directory/me", model.WorkProfile{}, 200)
	self, err := services.Auth.WorkProfile(t.Context(), alice.ID)
	if err != nil || self.Department != "" {
		t.Fatalf("clear failed: %+v %v", self, err)
	}
	// Colleagues can start direct conversations without first adding a friend.
	httpCall(t, server, aliceToken, "POST", "/api/v1/conversations", model.CreateInput{Kind: "direct", MemberIDs: []string{bob.ID}}, 201)
	// Search must reach beyond the old users endpoint's 1,000-account cap.
	_, err = services.Store.DB.Exec(t.Context(), `INSERT INTO users(id,email,name,password_hash)
		SELECT 'bulk-'||n, 'bulk-'||n||'@example.test', 'bulk-'||n, 'unused' FROM generate_series(1,1005) n`)
	if err != nil {
		t.Fatal(err)
	}
	page = decodePage("/api/v1/directory?q=bulk-1005@example.test")
	if len(page.Entries) != 1 || page.Entries[0].ID != "bulk-1005" {
		t.Fatalf("search truncated: %+v", page)
	}
}
