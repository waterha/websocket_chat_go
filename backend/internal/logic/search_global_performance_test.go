package logic

import (
	"encoding/json"
	"os"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
)

// Opt-in: this test builds an isolated 300k-message dataset, never user data.
func TestGlobalSearchPerformance(t *testing.T) {
	if os.Getenv("IM_TEST_SEARCH_PERFORMANCE") != "1" {
		t.Skip("set IM_TEST_SEARCH_PERFORMANCE=1 for the 300k-message benchmark")
	}
	messageCount := 300000
	if value := os.Getenv("IM_TEST_SEARCH_MESSAGES"); value != "" {
		n, e := strconv.Atoi(value)
		if e != nil || n < 30000 || n > 1000000 {
			t.Fatal("IM_TEST_SEARCH_MESSAGES must be 30000..1000000")
		}
		messageCount = n
	}
	chat := historyFixture(t)
	db := chat.Store.DB
	ctx := t.Context()
	must := func(sql string) {
		t.Helper()
		if _, e := db.Exec(ctx, sql); e != nil {
			t.Fatal(e)
		}
	}
	// Bulk load without the new indexes, then separately measure their build cost.
	must(strings.ReplaceAll(`DROP INDEX messages_global_search_grams_idx; DROP INDEX messages_global_search_recent_idx; DROP INDEX messages_global_search_conversation_idx; DROP INDEX conversation_members_search_user_idx;
 INSERT INTO users(id,email,name,password_hash) VALUES('broad','broad@bench','broad','unused'),('sparse','sparse@bench','sparse','unused'),('author','author@bench','author','unused');
 INSERT INTO conversations(id,kind,name,next_seq) SELECT 'bench-'||n,CASE WHEN n%5=0 THEN 'group' ELSE 'direct' END,'bench',30 FROM generate_series(1,10000)n;
 INSERT INTO conversation_members(conversation_id,user_id) SELECT 'bench-'||n,'author' FROM generate_series(1,10000)n;
 INSERT INTO conversation_members(conversation_id,user_id) SELECT 'bench-'||n,'broad' FROM generate_series(1,5000)n;
 INSERT INTO conversation_members(conversation_id,user_id) SELECT 'bench-'||n,'sparse' FROM generate_series(1,5)n;
 INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content,created_at)
 SELECT 'perf-'||lpad(n::text,9,'0'),'bench-'||(((n-1)%10000)+1),'author','perf-client-'||n,((n-1)/10000)+1,
 '日常项目的消息 common budget '||md5(n::text)||CASE WHEN n%4999=0 THEN '稀有预算needle-unique' ELSE '' END||CASE WHEN n%113=0 THEN ' %_\\ 😀' ELSE '' END,
 '2026-01-01'::timestamptz+n*interval '1 second' FROM generate_series(1,300000)n;
 ANALYZE messages; ANALYZE conversation_members;`, "300000", strconv.Itoa(messageCount)))
	started := time.Now()
	must(`CREATE INDEX messages_global_search_grams_idx ON messages USING gin(message_search_grams(content)) WHERE recalled_at IS NULL;
 CREATE INDEX messages_global_search_recent_idx ON messages(created_at DESC,id DESC) WHERE recalled_at IS NULL;
 CREATE INDEX messages_global_search_conversation_idx ON messages(conversation_id,created_at DESC,id DESC) WHERE recalled_at IS NULL;
 CREATE INDEX conversation_members_search_user_idx ON conversation_members(user_id,conversation_id); ANALYZE messages; ANALYZE conversation_members;`)
	t.Logf("DATASET messages=%d conversations=10000 broad=5000 sparse=5 index_build=%s", messageCount, time.Since(started))
	sizes, e := db.Query(ctx, `SELECT indexrelname,pg_relation_size(indexrelid) FROM pg_stat_user_indexes WHERE relid='messages'::regclass AND indexrelname LIKE 'messages_global_search_%' ORDER BY indexrelname`)
	if e != nil {
		t.Fatal(e)
	}
	for sizes.Next() {
		var name string
		var size int64
		if e = sizes.Scan(&name, &size); e != nil {
			t.Fatal(e)
		}
		t.Logf("INDEX %s bytes=%d", name, size)
	}
	sizes.Close()
	baseline := `SELECT m.id FROM messages m JOIN conversation_members mine ON mine.conversation_id=m.conversation_id AND mine.user_id=$1 WHERE m.recalled_at IS NULL AND strpos(lower(m.content),lower($2::text))>0 ORDER BY m.created_at DESC,m.id DESC LIMIT $3`
	explain := func(sql, user, q string) (float64, string) {
		t.Helper()
		var raw []byte
		e := db.QueryRow(ctx, "EXPLAIN (ANALYZE,BUFFERS,FORMAT JSON) "+sql, pgx.QueryExecModeExec, user, q, 21).Scan(&raw)
		if e != nil {
			t.Fatal(e)
		}
		var plan []map[string]any
		if e = json.Unmarshal(raw, &plan); e != nil {
			t.Fatal(e)
		}
		return plan[0]["Execution Time"].(float64), string(raw)
	}
	for _, user := range []string{"broad", "sparse"} {
		for _, q := range []string{"的", "预算", "needle-unique", "不存在的编号", "%_", "😀"} {
			t.Run(user+"/"+q, func(t *testing.T) {
				rows, e := db.Query(ctx, baseline, pgx.QueryExecModeExec, user, q, 21)
				if e != nil {
					t.Fatal(e)
				}
				expected := []string{}
				for rows.Next() {
					var id string
					if e = rows.Scan(&id); e != nil {
						t.Fatal(e)
					}
					expected = append(expected, id)
				}
				rows.Close()
				if e = rows.Err(); e != nil {
					t.Fatal(e)
				}
				page, e := chat.SearchAll(ctx, user, q, nil, "", 20)
				if e != nil {
					t.Fatal(e)
				}
				actual := []string{}
				for _, hit := range page.Results {
					actual = append(actual, hit.Message.ID)
				}
				want := expected
				if len(want) > 20 {
					want = want[:20]
				}
				if !reflect.DeepEqual(actual, want) || page.HasMore != (len(expected) > 20) {
					t.Fatalf("result mismatch %v %v", actual, want)
				}
				if page.HasMore {
					next, e := chat.SearchAll(ctx, user, q, &page.NextBeforeTime, page.NextBeforeID, 20)
					if e != nil {
						t.Fatal(e)
					}
					if len(next.Results) == 0 || next.Results[0].Message.ID != expected[20] {
						t.Fatal("cursor mismatch")
					}
				}
				oldTimes, newTimes := []float64{}, []float64{}
				var plan string
				for i := 0; i < 3; i++ {
					old, _ := explain(baseline, user, q)
					now, p := explain(globalSearchSQL(false), user, q)
					oldTimes = append(oldTimes, old)
					newTimes = append(newTimes, now)
					plan = p
				}
				sort.Float64s(oldTimes)
				sort.Float64s(newTimes)
				t.Logf("TIMING old_candidate_ms median=%.3f range=%.3f..%.3f new_full_ms median=%.3f range=%.3f..%.3f gin=%t recency=%t conv=%t", oldTimes[1], oldTimes[0], oldTimes[2], newTimes[1], newTimes[0], newTimes[2], strings.Contains(plan, "messages_global_search_grams_idx"), strings.Contains(plan, "messages_global_search_recent_idx"), strings.Contains(plan, "messages_global_search_conversation_idx"))
				if q == "的" || q == "needle-unique" || q == "不存在的编号" {
					t.Logf("PLAN %s", plan)
				}
			})
		}
	}
	// Measure long Unicode writes with and without only the new indexes, rolling
	// back every sample so the same isolated fixture can compare both cases.
	for _, indexed := range []bool{true, false} {
		tx, e := db.Begin(ctx)
		if e != nil {
			t.Fatal(e)
		}
		if !indexed {
			_, e = tx.Exec(ctx, `DROP INDEX messages_global_search_grams_idx; DROP INDEX messages_global_search_recent_idx; DROP INDEX messages_global_search_conversation_idx;`)
			if e != nil {
				t.Fatal(e)
			}
		}
		start := time.Now()
		_, e = tx.Exec(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content) SELECT 'long-'||n,'bench-1','author','long-'||n,100+n,left(repeat(md5(n::text)||'中文项目测试',110),4000) FROM generate_series(1,128)n`)
		if e != nil {
			t.Fatal(e)
		}
		t.Logf("WRITE long_messages=128 chars=4000 indexed=%t elapsed=%s", indexed, time.Since(start))
		if e = tx.Rollback(ctx); e != nil {
			t.Fatal(e)
		}
	}
	t.Log("performance evidence collected from an isolated schema")
}
