package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"websocket_chat_go/backend/internal/model"
)

func TestSearchMessagesRejectsMalformedPagination(t *testing.T) {
	for _, query := range []string{"before_seq=", "before_seq=0", "before_seq=-1", "before_seq=no", "before_seq=9223372036854775808", "limit=", "limit=0", "limit=-1", "limit=101", "limit=no", "limit=9223372036854775808"} {
		t.Run(query, func(t *testing.T) {
			request := httptest.NewRequest("GET", "/?q=test&"+query, nil)
			writer := httptest.NewRecorder()
			(&Server{}).searchMessages(writer, request, identity{})
			if writer.Code != 400 {
				t.Fatalf("wanted 400, got %d: %s", writer.Code, writer.Body.String())
			}
			code := "invalid_limit"
			if strings.HasPrefix(query, "before_seq=") {
				code = "invalid_cursor"
			}
			if !strings.Contains(writer.Body.String(), code) {
				t.Fatalf("wrong error code: %s", writer.Body.String())
			}
		})
	}
}

func TestSearchMessagesHTTP(t *testing.T) {
	services := fixture(t, false)
	user, token := account(t, services, "search-member")
	_, outsiderToken := account(t, services, "search-outsider")
	conversation, err := services.Chat.Create(t.Context(), user.ID, model.CreateInput{Kind: "group", Name: "search"})
	if err != nil {
		t.Fatal(err)
	}
	for index, content := range []string{"MiXeD 100%_value", "mixed 100xxvalue", "MIXED 100%_value"} {
		_, _, err := services.Chat.Send(t.Context(), user.ID, model.SendInput{ConversationID: conversation, ClientMsgID: "search-msg-" + string(rune('a'+index)), Content: content})
		if err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	path := "/api/v1/conversations/" + conversation + "/messages/search"
	httpCall(t, server, "", http.MethodGet, path+"?q=mixed", nil, 401)
	httpCall(t, server, outsiderToken, http.MethodGet, path+"?q=mixed", nil, 404)
	httpCall(t, server, token, http.MethodGet, "/api/v1/conversations/missing/messages/search?q=mixed", nil, 404)
	for _, query := range []string{"q=" + url.QueryEscape(strings.Repeat("界", 101)), "q=test&before_seq=0", "q=test&limit=101"} {
		httpCall(t, server, token, http.MethodGet, path+"?"+query, nil, 400)
	}
	var page model.SearchPage
	for _, query := range []string{"", "q=", "q=%20%09", "type=all", "sender_id=" + user.ID} {
		data := httpCall(t, server, token, http.MethodGet, path+"?"+query, nil, 200)
		if err := json.Unmarshal(data, &page); err != nil || len(page.Messages) != 3 || page.HasMore {
			t.Fatalf("browse %q: %s %v", query, data, err)
		}
	}
	data := httpCall(t, server, token, http.MethodGet, path+"?q="+url.QueryEscape("  MIXED  "), nil, 200)
	if err := json.Unmarshal(data, &page); err != nil || len(page.Messages) != 3 || page.HasMore || page.NextBeforeSeq != 1 {
		t.Fatalf("default pagination: %s %v", data, err)
	}
	data = httpCall(t, server, token, http.MethodGet, path+"?q=%25_&limit=1", nil, 200)
	if err := json.Unmarshal(data, &page); err != nil || len(page.Messages) != 1 || page.Messages[0].Seq != 3 || !page.HasMore || page.NextBeforeSeq != 3 {
		t.Fatalf("first page: %s %v", data, err)
	}
	data = httpCall(t, server, token, http.MethodGet, path+"?q=%25_&limit=1&before_seq=3", nil, 200)
	if err := json.Unmarshal(data, &page); err != nil || len(page.Messages) != 1 || page.Messages[0].Seq != 1 || page.HasMore || page.NextBeforeSeq != 1 {
		t.Fatalf("last page: %s %v", data, err)
	}
	data = httpCall(t, server, token, http.MethodGet, path+"?q=mixed&before_seq=1", nil, 200)
	if err := json.Unmarshal(data, &page); err != nil || page.Messages == nil || len(page.Messages) != 0 || page.HasMore || page.NextBeforeSeq != 0 {
		t.Fatalf("empty page: %s %v", data, err)
	}
}

func TestSearchMessagesRejectsMalformedFilters(t *testing.T) {
	for _, test := range []struct {
		query string
		code  string
	}{
		{"type=video", "invalid_type"},
		{"from=", "invalid_from"},
		{"to=", "invalid_to"},
		{"from=2026-01-02", "invalid_from"},
		{"to=not-a-date", "invalid_to"},
		{"from=2026-02-30T00:00:00Z", "invalid_from"},
		{"from=2026-01-03T00:00:00Z&to=2026-01-02T00:00:00Z", "invalid_time_range"},
		{"from=2026-01-02T00:00:00Z&to=2026-01-02T08:00:00%2B08:00", "invalid_time_range"},
	} {
		t.Run(test.query, func(t *testing.T) {
			writer := httptest.NewRecorder()
			(&Server{}).searchMessages(writer, httptest.NewRequest("GET", "/?"+test.query, nil), identity{})
			if writer.Code != 400 || !strings.Contains(writer.Body.String(), test.code) {
				t.Fatalf("wanted 400 %s, got %d: %s", test.code, writer.Code, writer.Body.String())
			}
		})
	}
}

func TestSearchMessagesHTTPFilters(t *testing.T) {
	services := fixture(t, false)
	user, token := account(t, services, "filter-member")
	outsider, outsiderToken := account(t, services, "filter-outsider")
	conversation, err := services.Chat.Create(t.Context(), user.ID, model.CreateInput{Kind: "group", Name: "filters"})
	if err != nil {
		t.Fatal(err)
	}
	for index := range 5 {
		_, _, err := services.Chat.Send(t.Context(), user.ID, model.SendInput{ConversationID: conversation, ClientMsgID: "filter-" + string(rune('a'+index)), Content: "plain"})
		if err != nil {
			t.Fatal(err)
		}
	}
	if _, err := services.Store.DB.Exec(t.Context(), `UPDATE messages SET created_at='2026-01-02T03:04:05.123456Z'::timestamptz+(seq-1)*interval '1 second' WHERE conversation_id=$1`, conversation); err != nil {
		t.Fatal(err)
	}
	for _, update := range []string{
		`UPDATE messages SET blocks='[{"type":"image","media_type":"sticker"}]' WHERE conversation_id=$1 AND seq=2`,
		`UPDATE messages SET kind='file',blocks='[{"type":"text","text":"file caption"}]' WHERE conversation_id=$1 AND seq=3`,
		`UPDATE messages SET blocks='[{"type":"text","text":"https://example.test/100%_path"}]',sender_id=$2 WHERE conversation_id=$1 AND seq=4`,
		`UPDATE messages SET kind='image',blocks='[{"type":"image"}]',content='secret www.secret.test',recalled_at=now() WHERE conversation_id=$1 AND seq=5`,
	} {
		args := []any{conversation}
		if strings.Contains(update, "$2") {
			args = append(args, outsider.ID)
		}
		if _, err := services.Store.DB.Exec(t.Context(), update, args...); err != nil {
			t.Fatal(err)
		}
	}
	server := httptest.NewServer((&Server{Services: services}).Handler())
	t.Cleanup(server.Close)
	path := "/api/v1/conversations/" + conversation + "/messages/search"
	for _, test := range []struct {
		query string
		seq   int64
	}{
		{"type=images", 2},
		{"type=files", 3},
		{"type=links", 4},
		{"sender_id=" + outsider.ID, 4},
		{"from=2026-01-02T11%3A04%3A06.123456%2B08%3A00&to=2026-01-02T03%3A04%3A07.123456Z", 2},
		{"type=images&sender_id=" + user.ID + "&from=2026-01-02T03:04:06.123456000Z&to=2026-01-02T03:04:07.123456000Z", 2},
	} {
		data := httpCall(t, server, token, http.MethodGet, path+"?"+test.query, nil, 200)
		var page model.SearchPage
		if err := json.Unmarshal(data, &page); err != nil || len(page.Messages) != 1 || page.Messages[0].Seq != test.seq || page.HasMore {
			t.Fatalf("filter %s: %s %v", test.query, data, err)
		}
	}
	for _, query := range []string{"", "type=images", "sender_id=" + user.ID} {
		httpCall(t, server, outsiderToken, http.MethodGet, path+"?"+query, nil, 404)
	}
	for _, query := range []string{"q=secret", "type=links&sender_id=" + user.ID, "type=images&before_seq=2", "sender_id=missing"} {
		data := httpCall(t, server, token, http.MethodGet, path+"?"+query, nil, 200)
		var page model.SearchPage
		if err := json.Unmarshal(data, &page); err != nil || len(page.Messages) != 0 || page.HasMore {
			t.Fatalf("empty filter %s: %s %v", query, data, err)
		}
	}
}
