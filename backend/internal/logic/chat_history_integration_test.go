package logic

import (
	"context"
	"errors"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/model"
)

// History storage tests need PostgreSQL only, independently of realtime/Redis.
func historyFixture(t *testing.T) *Chat {
	t.Helper()
	url := os.Getenv("IM_TEST_DATABASE_URL")
	if url == "" {
		t.Skip("set IM_TEST_DATABASE_URL for history storage integration tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "im_history_" + model.NewID()
	if _, err := admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	config, err := pgxpool.ParseConfig(url)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	store := &model.Store{DB: db}
	if err := store.Migrate(ctx, filepath.Join("..", "..", "..", "database", "migrations")); err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(ctx, `
		INSERT INTO users(id,email,name,password_hash) VALUES('member','member@test','member','unused'),('outsider','outsider@test','outsider','unused');
		INSERT INTO conversations(id,kind,name,next_seq) VALUES('history','group','history',1205),('other','group','other',1);
		INSERT INTO conversation_members(conversation_id,user_id) VALUES('history','member'),('other','member');
		INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content)
		SELECT 'seed-'||n,'history','member','seed-msg-'||n,n,'message '||n FROM generate_series(1,1205) n;
		INSERT INTO messages(id,conversation_id,sender_id,client_msg_id,seq,content) VALUES('other-msg','other','member','other-client',1,'other conversation');
		ANALYZE messages;
	`)
	if err != nil {
		t.Fatal(err)
	}
	return &Chat{Store: store}
}

func TestHistoryStoragePaginationDuringAppends(t *testing.T) {
	chat := historyFixture(t)
	ctx := t.Context()
	before, expected := int64(math.MaxInt64), int64(1205)
	for page := 0; ; page++ {
		items, err := chat.History(ctx, "member", "history", before, 37)
		if err != nil {
			t.Fatal(err)
		}
		if len(items) == 0 {
			break
		}
		for index := len(items) - 1; index >= 0; index-- {
			if items[index].ConversationID != "history" || items[index].Seq != expected {
				t.Fatalf("duplicate, gap or foreign message: expected %d, got %+v", expected, items[index])
			}
			expected--
		}
		before = items[0].Seq
		// New writes between page requests must not shift the old-page cursor.
		if _, _, err := chat.Send(ctx, "member", model.SendInput{ConversationID: "history", ClientMsgID: fmt.Sprintf("append-%04d", page), Content: "new while paging"}); err != nil {
			t.Fatal(err)
		}
	}
	if expected != 0 {
		t.Fatalf("history truncated at %d", expected)
	}
	after := int64(0)
	for {
		page, err := chat.Sync(ctx, "member", "history", after, 100)
		if err != nil {
			t.Fatal(err)
		}
		for _, message := range page.Messages {
			if message.ConversationID != "history" || message.Seq != after+1 {
				t.Fatalf("sync gap at %d: %+v", after, message)
			}
			after = message.Seq
		}
		if page.NextSeq != after || page.HasMore != (after < page.LastSeq) {
			t.Fatalf("invalid page: %+v", page)
		}
		if !page.HasMore {
			break
		}
	}
	if after <= 1205 {
		t.Fatal("sync did not include appended messages")
	}
	items, err := chat.History(ctx, "member", "history", 1, 100)
	if err != nil || len(items) != 0 {
		t.Fatalf("earliest boundary: %+v %v", items, err)
	}
	page, err := chat.Sync(ctx, "member", "history", after, 100)
	if err != nil || len(page.Messages) != 0 || page.HasMore || page.NextSeq != after {
		t.Fatalf("latest boundary: %+v %v", page, err)
	}
	for _, query := range []func() error{
		func() error { _, err := chat.History(ctx, "outsider", "history", 100, 30); return err },
		func() error { _, err := chat.Sync(ctx, "outsider", "history", 0, 30); return err },
	} {
		var failure *model.Error
		if err := query(); !errors.As(err, &failure) || failure.Status != 404 {
			t.Fatalf("history exposed to non-member: %v", err)
		}
	}
}

func TestHistoryStorageIndexMigrationAndPlans(t *testing.T) {
	chat := historyFixture(t)
	ctx, db := t.Context(), chat.Store.DB
	var indexName *string
	if err := db.QueryRow(ctx, "SELECT to_regclass('messages_history_idx')::text").Scan(&indexName); err != nil || indexName != nil {
		t.Fatalf("redundant index remains: %v %v", indexName, err)
	}
	var uniqueIndex string
	if err := db.QueryRow(ctx, `SELECT conindid::regclass::text FROM pg_constraint WHERE conrelid='messages'::regclass AND contype='u' AND pg_get_constraintdef(oid)='UNIQUE (conversation_id, seq)'`).Scan(&uniqueIndex); err != nil {
		t.Fatal("message sequence uniqueness lost", err)
	}
	for _, query := range []struct {
		direction string
		filter    string
		cursor    int64
	}{
		{"ASC", "seq>$2 AND seq<=$3", 500},
		{"DESC", "seq<$2 AND seq<=$3", 900},
	} {
		direction := query.direction
		rows, err := db.Query(ctx, "EXPLAIN (ANALYZE, BUFFERS) SELECT "+messageColumns+" FROM messages WHERE conversation_id=$1 AND "+query.filter+" ORDER BY seq "+direction+" LIMIT 10", "history", query.cursor, int64(1205))
		if err != nil {
			t.Fatal(err)
		}
		var lines []string
		for rows.Next() {
			var line string
			if err := rows.Scan(&line); err != nil {
				t.Fatal(err)
			}
			lines = append(lines, line)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			t.Fatal(err)
		}
		plan := strings.Join(lines, "\n")
		t.Logf("%s default planner:\n%s", direction, plan)
		if !strings.Contains(plan, uniqueIndex) || strings.Contains(plan, "Sort") {
			t.Fatalf("expected ordered unique-index range scan: %s", plan)
		}
		if direction == "DESC" && !strings.Contains(plan, "Index Scan Backward") {
			t.Fatalf("expected reverse index scan: %s", plan)
		}
	}
	// Exercise the supplied rollback and upgrade against populated data.
	for _, suffix := range []string{"down", "up"} {
		path := filepath.Join("..", "..", "..", "database", "migrations", "000006_message_history_index."+suffix+".sql")
		sql, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := db.Exec(ctx, string(sql)); err != nil {
			t.Fatal(err)
		}
		if err := db.QueryRow(ctx, "SELECT to_regclass('messages_history_idx')::text").Scan(&indexName); err != nil || (indexName != nil) != (suffix == "down") {
			t.Fatalf("index %s state: %v %v", suffix, indexName, err)
		}
		if suffix == "down" {
			if _, err := db.Exec(ctx, "DELETE FROM schema_migrations WHERE version='000006_message_history_index'"); err != nil {
				t.Fatal(err)
			}
			if err := chat.Store.Migrate(ctx, filepath.Join("..", "..", "..", "database", "migrations")); err != nil {
				t.Fatal("upgrade populated database", err)
			}
		}
	}
	if err := chat.Store.Migrate(ctx, filepath.Join("..", "..", "..", "database", "migrations")); err != nil {
		t.Fatal("migration is not repeatable", err)
	}
	var count int
	if err := db.QueryRow(ctx, "SELECT count(*) FROM messages").Scan(&count); err != nil || count != 1206 {
		t.Fatalf("migration lost history: %d %v", count, err)
	}
}
