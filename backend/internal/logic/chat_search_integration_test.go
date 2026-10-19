package logic

import (
	"errors"
	"math"
	"reflect"
	"strings"
	"testing"

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
	}{{"", 1, 30, "invalid_query"}, {" \t\n", 1, 30, "invalid_query"}, {strings.Repeat("界", 101), 1, 30, "invalid_query"}, {"a", 0, 30, "invalid_cursor"}, {"a", -1, 30, "invalid_cursor"}, {"a", 1, 0, "invalid_limit"}, {"a", 1, 101, "invalid_limit"}} {
		_, err := chat.SearchMessages(ctx, "member", "history", input.query, input.before, input.limit)
		var failure *model.Error
		if !errors.As(err, &failure) || failure.Status != 400 || failure.Code != input.code {
			t.Fatalf("invalid input %+v: %v", input, err)
		}
	}
}
