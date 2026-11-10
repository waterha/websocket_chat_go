package logic

import (
	"fmt"
	"reflect"
	"testing"
	"time"
)

func TestGlobalSearchFindsOlderMatchesAcrossConversations(t *testing.T) {
	chat := historyFixture(t)
	ctx := t.Context()
	db := chat.Store.DB
	_, err := db.Exec(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,created_at)
 SELECT 'probe-'||n,'history','member','probe-client-'||n,2000+n,CASE WHEN n<=72 THEN 'boundary-key' ELSE 'unmatched' END,'2026-01-01 08:00Z'::timestamptz+n*interval '1 second' FROM generate_series(1,200)n;
 INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,created_at)
 SELECT 'older-'||n,'other','member','older-client-'||n,2000+n,'boundary-key','2026-01-01 07:00Z'::timestamptz+n*interval '1 second' FROM generate_series(1,30)n;`)
	if err != nil {
		t.Fatal(err)
	}
	page, err := chat.SearchAll(ctx, "member", "boundary-key", nil, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 20 || !page.HasMore {
		t.Fatalf("missing hits: %+v", page)
	}
	for i, hit := range page.Results {
		if hit.Message.ID != fmt.Sprintf("probe-%d", 72-i) {
			t.Fatalf("truncated newer match: %s", hit.Message.ID)
		}
	}
	next, err := chat.SearchAll(ctx, "member", "boundary-key", &page.NextBeforeTime, page.NextBeforeID, 20)
	if err != nil {
		t.Fatal(err)
	}
	if next.Results[0].Message.ID != "probe-52" {
		t.Fatalf("probe cursor skipped hits: %+v", next)
	}
}

func TestGlobalIndexedSearchLiteralUnicodeAndPagination(t *testing.T) {
	chat := historyFixture(t)
	db := chat.Store.DB
	ctx := t.Context()
	_, err := db.Exec(ctx, `INSERT INTO conversations(id,kind,name,next_seq) VALUES('hidden-search','group','hidden',10);
 INSERT INTO conversation_members(conversation_id,user_id) VALUES('hidden-search','outsider');`)
	if err != nil {
		t.Fatal(err)
	}
	bodies := []string{"季度预算的 ABC 100%_\\ 😀 👩🏽‍💻 !?.", "预算的 abc", "abc xxx bcd", "literal abcd", "季度预算的 ABC 100%_\\ 😀 👩🏽‍💻 !?."}
	for i, body := range bodies {
		cid, sender := "history", "member"
		if i == 4 {
			cid, sender = "hidden-search", "outsider"
		}
		if i == 1 {
			cid = "other"
		}
		_, err = db.Exec(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,created_at) VALUES($1,$2,$3,$1,$4,$5,'2026-11-10T00:00:00Z')`, fmt.Sprintf("indexed-%d", i), cid, sender, 1300+i, body)
		if err != nil {
			t.Fatal(err)
		}
	}
	for _, term := range []string{"的", "预算", "季度预算", "ABC", "%", "_", "\\", "😀", "👩🏽‍💻", "!?.", "abcd", "不存在的词"} {
		t.Run(term, func(t *testing.T) {
			rows, err := db.Query(ctx, `SELECT m.id FROM messages m JOIN conversation_members mine ON mine.conversation_id=m.conversation_id AND mine.user_id='member' WHERE m.recalled_at IS NULL AND strpos(lower(m.content),lower($1))>0 ORDER BY m.created_at DESC,m.id DESC`, term)
			if err != nil {
				t.Fatal(err)
			}
			expected := []string{}
			for rows.Next() {
				var id string
				if err = rows.Scan(&id); err != nil {
					t.Fatal(err)
				}
				expected = append(expected, id)
			}
			rows.Close()
			if err = rows.Err(); err != nil {
				t.Fatal(err)
			}
			actual := []string{}
			var cursor *time.Time
			beforeID := ""
			for pageNumber := 0; pageNumber < 20; pageNumber++ {
				page, e := chat.SearchAll(ctx, "member", term, cursor, beforeID, 1)
				if e != nil {
					t.Fatal(e)
				}
				for _, hit := range page.Results {
					actual = append(actual, hit.Message.ID)
					if hit.MemberCount != 1 {
						t.Fatalf("bad member count: %+v", hit)
					}
				}
				if !page.HasMore {
					break
				}
				next := page.NextBeforeTime
				cursor = &next
				beforeID = page.NextBeforeID
				if pageNumber == 19 {
					t.Fatal("cursor did not advance")
				}
			}
			if !reflect.DeepEqual(actual, expected) {
				t.Fatalf("literal matching/order changed for %q: %v != %v", term, actual, expected)
			}
		})
	}
	if _, err = db.Exec(ctx, `UPDATE messages SET recalled_at=now() WHERE id='indexed-0'`); err != nil {
		t.Fatal(err)
	}
	page, err := chat.SearchAll(ctx, "member", "季度预算", nil, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 0 {
		t.Fatalf("recalled/foreign result leaked: %+v", page)
	}
	// Membership is checked on every page, never cached with search results.
	if _, err = db.Exec(ctx, `DELETE FROM conversation_members WHERE conversation_id='other' AND user_id='member'`); err != nil {
		t.Fatal(err)
	}
	page, err = chat.SearchAll(ctx, "member", "预算", nil, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	if len(page.Results) != 0 {
		t.Fatalf("former member result leaked: %+v", page)
	}
	for _, input := range []struct {
		q     string
		limit int
	}{{"", 1}, {"budget", 0}, {"budget", 51}} {
		_, err = chat.SearchAll(ctx, "member", input.q, nil, "", input.limit)
		if err == nil {
			t.Fatalf("expected validation for %+v", input)
		}
	}
}
