package logic

import "testing"

func TestGlobalSearchFrequentAndMissingStatistics(t *testing.T) {
	chat := historyFixture(t)
	db := chat.Store.DB
	ctx := t.Context()
	_, err := db.Exec(ctx, `INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content) SELECT 'stat-'||n,'history','member','stat-client-'||n,3000+n,'日常项目的消息 common budget '||md5(n::text) FROM generate_series(1,3000)n; ANALYZE messages;`)
	if err != nil {
		t.Fatal(err)
	}
	var hot bool
	err = db.QueryRow(ctx, `SELECT COALESCE(most_common_elem_freqs[array_position(most_common_elems::text::text[],'的')],0)>=0.2 FROM pg_stats WHERE schemaname=current_schema() AND tablename='messages_global_search_grams_idx'`).Scan(&hot)
	if err != nil || !hot {
		t.Fatalf("frequent Unicode term statistics missing: hot=%v err=%v", hot, err)
	}
	first, err := chat.SearchAll(ctx, "member", "的", nil, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(first.Results) != 20 || !first.HasMore {
		t.Fatalf("hot term results missing: %+v", first)
	}
	// Statistics choose only an execution strategy. Missing/outdated statistics
	// must never truncate, reorder or change literal matching.
	_, err = db.Exec(ctx, `DROP INDEX messages_global_search_grams_idx; CREATE INDEX messages_global_search_grams_idx ON messages USING gin(message_search_grams(content)) WHERE recalled_at IS NULL;`)
	if err != nil {
		t.Fatal(err)
	}
	second, err := chat.SearchAll(ctx, "member", "的", nil, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(second.Results) != len(first.Results) || second.HasMore != first.HasMore {
		t.Fatal("statistics changed results")
	}
	for i := range first.Results {
		if first.Results[i].Message.ID != second.Results[i].Message.ID {
			t.Fatal("statistics changed ordering")
		}
	}
}
