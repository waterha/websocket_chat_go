package logic

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"
	"time"

	"websocket_chat_go/backend/internal/model"
)

func TestSearchMessagesStorage(t *testing.T) {
	chat := historyFixture(t)
	ctx := t.Context()
	_, err := chat.Store.DB.Exec(ctx, `UPDATE messages SET content=CASE seq
		WHEN 1 THEN 'Alpha 100%_done'
		WHEN 2 THEN 'alpha 100xxdone'
		WHEN 3 THEN 'ALPHA 100%_done'
		WHEN 4 THEN 'alpha 100%_DONE'
		WHEN 5 THEN 'path\name'
		ELSE content END WHERE conversation_id='history';
		UPDATE messages SET content='ALPHA 100%_done' WHERE conversation_id='other'`)
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		query string
		want  []int64
	}{
		{"  aLpHa  ", []int64{4, 3, 2, 1}},
		{"%_", []int64{4, 3, 1}},
		{"_", []int64{4, 3, 1}},
		{"%", []int64{4, 3, 1}},
		{`\`, []int64{5}},
		{"not present", []int64{}},
		{strings.Repeat("界", 100), []int64{}},
	} {
		t.Run(test.query, func(t *testing.T) {
			page, err := chat.SearchMessages(ctx, "member", "history", test.query, math.MaxInt64, 100)
			if err != nil {
				t.Fatal(err)
			}
			got := []int64{}
			for _, message := range page.Messages {
				if message.ConversationID != "history" {
					t.Fatalf("foreign conversation: %+v", message)
				}
				got = append(got, message.Seq)
			}
			if !reflect.DeepEqual(got, test.want) || page.HasMore || page.Messages == nil {
				t.Fatalf("got %+v, want %v", page, test.want)
			}
		})
	}
	before := int64(math.MaxInt64)
	for index, want := range []struct {
		seqs []int64
		more bool
		next int64
	}{{[]int64{4, 3}, true, 3}, {[]int64{1}, false, 1}, {[]int64{}, false, 0}} {
		page, err := chat.SearchMessages(ctx, "member", "history", "%_", before, 2)
		if err != nil {
			t.Fatal(err)
		}
		got := []int64{}
		for _, item := range page.Messages {
			got = append(got, item.Seq)
		}
		if !reflect.DeepEqual(got, want.seqs) || page.HasMore != want.more || page.NextBeforeSeq != want.next {
			t.Fatalf("page %d: %+v want %+v", index, page, want)
		}
		before = page.NextBeforeSeq
	}
	page, err := chat.SearchMessages(ctx, "member", "history", "alpha", math.MaxInt64, 4)
	if err != nil || page.HasMore || len(page.Messages) != 4 {
		t.Fatalf("exactly full final page: %+v %v", page, err)
	}
	for _, userConversation := range [][2]string{{"outsider", "history"}, {"member", "missing"}} {
		_, err := chat.SearchMessages(ctx, userConversation[0], userConversation[1], "alpha", math.MaxInt64, 30)
		var failure *model.Error
		if !errors.As(err, &failure) || failure.Status != 404 {
			t.Fatalf("unauthorized search: %v", err)
		}
	}
	for _, input := range []struct {
		query  string
		before int64
		limit  int
		code   string
	}{{strings.Repeat("界", 101), 1, 30, "invalid_query"}, {"a", 0, 30, "invalid_cursor"}, {"a", -1, 30, "invalid_cursor"}, {"a", 1, 0, "invalid_limit"}, {"a", 1, 101, "invalid_limit"}} {
		_, err := chat.SearchMessages(ctx, "member", "history", input.query, input.before, input.limit)
		var failure *model.Error
		if !errors.As(err, &failure) || failure.Status != 400 || failure.Code != input.code {
			t.Fatalf("invalid input %+v: %v", input, err)
		}
	}
}

func TestSearchMessagesFiltersStorage(t *testing.T) {
	chat := historyFixture(t)
	ctx := t.Context()
	_, err := chat.Store.DB.Exec(ctx, `DELETE FROM messages WHERE conversation_id='history' AND seq>12;
		UPDATE messages SET created_at='2026-01-02T03:04:05.123456Z'::timestamptz + (seq-1)*interval '1 second',
			content='plain', blocks='[]'::jsonb WHERE conversation_id='history';
		UPDATE messages SET kind='image',blocks='[{"type":"text","text":"legacy image caption"}]' WHERE conversation_id='history' AND seq IN (2,9);
		UPDATE messages SET blocks='[{"type":"text","text":"caption"},{"type":"image","media_type":"sticker"}]' WHERE conversation_id='history' AND seq=3;
		UPDATE messages SET kind='file',blocks='[{"type":"text","text":"file caption"}]' WHERE conversation_id='history' AND seq IN (4,10);
		UPDATE messages SET content='visit HTTP://example.test/100%_path' WHERE conversation_id='history' AND seq IN (5,11);
		UPDATE messages SET content='visit https://example.test' WHERE conversation_id='history' AND seq=6;
		UPDATE messages SET blocks='[{"type":"text","text":"WWW.example.test"},{"type":"image","file_name":"https://metadata.test"}]' WHERE conversation_id='history' AND seq=7;
		UPDATE messages SET blocks='[{"type":"image","file_name":"https://metadata.test"}]' WHERE conversation_id='history' AND seq=8;
		UPDATE messages SET recalled_at=now(), blocks='[{"type":"image"},{"type":"text","text":"www.secret.test"}]',content='secret http://secret.test' WHERE conversation_id='history' AND seq BETWEEN 9 AND 11;
		UPDATE messages SET sender_id='outsider' WHERE conversation_id='history' AND seq IN (3,7);
		UPDATE messages SET content='foreign http://example.test' WHERE conversation_id='other'`)
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2026, 1, 2, 3, 4, 7, 123456000, time.UTC)
	to := from.Add(4 * time.Second)
	nanoFrom, nanoTo := from.Add(time.Nanosecond), to.Add(time.Nanosecond)
	for _, test := range []struct {
		name    string
		query   string
		filters SearchFilters
		want    []int64
	}{
		{"all", "", SearchFilters{}, []int64{12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}},
		{"whitespace", " \t\n", SearchFilters{Type: "all"}, []int64{12, 11, 10, 9, 8, 7, 6, 5, 4, 3, 2, 1}},
		{"images", "", SearchFilters{Type: "images"}, []int64{8, 7, 3, 2}},
		{"files", "", SearchFilters{Type: "files"}, []int64{4}},
		{"links", "", SearchFilters{Type: "links"}, []int64{7, 6, 5}},
		{"sender", "", SearchFilters{SenderID: "outsider"}, []int64{7, 3}},
		{"unknown sender", "", SearchFilters{SenderID: "missing"}, []int64{}},
		{"dates", "", SearchFilters{From: &from, To: &to}, []int64{6, 5, 4, 3}},
		{"nanosecond dates", "", SearchFilters{From: &nanoFrom, To: &nanoTo}, []int64{7, 6, 5, 4}},
		{"from only", "", SearchFilters{From: &to}, []int64{12, 11, 10, 9, 8, 7}},
		{"to only", "", SearchFilters{To: &from}, []int64{2, 1}},
		{"combined", "", SearchFilters{SenderID: "outsider", Type: "images", From: &from, To: &to}, []int64{3}},
		{"literal keyword and type", "%_", SearchFilters{Type: "links"}, []int64{5}},
		{"recall keyword", "secret", SearchFilters{}, []int64{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			before := int64(math.MaxInt64)
			got := []int64{}
			for {
				page, err := chat.SearchMessages(ctx, "member", "history", test.query, before, 2, test.filters)
				if err != nil {
					t.Fatal(err)
				}
				for _, item := range page.Messages {
					got = append(got, item.Seq)
					if item.RecalledAt != nil && (item.Content != "" || item.Blocks != nil || item.MediaID != "") {
						t.Fatalf("recalled body leaked: %+v", item)
					}
				}
				if !page.HasMore {
					break
				}
				if page.NextBeforeSeq >= before || len(got) > 12 {
					t.Fatalf("nonadvancing cursor: %+v", page)
				}
				before = page.NextBeforeSeq
			}
			if !reflect.DeepEqual(got, test.want) {
				t.Fatalf("got %v, want %v", got, test.want)
			}
		})
	}
	for _, filter := range []SearchFilters{{Type: "video"}, {From: &to, To: &from}, {From: &from, To: &from}} {
		_, err := chat.SearchMessages(ctx, "member", "history", "", math.MaxInt64, 30, filter)
		var failure *model.Error
		if !errors.As(err, &failure) || failure.Status != 400 {
			t.Fatalf("invalid filter %+v: %v", filter, err)
		}
	}
	for _, filters := range []SearchFilters{{}, {Type: "images"}, {SenderID: "member", From: &from}} {
		_, err := chat.SearchMessages(ctx, "outsider", "history", "", math.MaxInt64, 30, filters)
		var failure *model.Error
		if !errors.As(err, &failure) || failure.Status != 404 {
			t.Fatalf("unauthorized filtered search: %v", err)
		}
	}
}
