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
	for _, query := range []string{"", "q=", "q=%20%09", "q=" + url.QueryEscape(strings.Repeat("界", 101)), "q=test&before_seq=0", "q=test&limit=101"} {
		httpCall(t, server, token, http.MethodGet, path+"?"+query, nil, 400)
	}
	var page model.SearchPage
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
