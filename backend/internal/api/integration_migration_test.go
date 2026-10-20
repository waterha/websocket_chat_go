package api

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"websocket_chat_go/backend/internal/model"
)

// TestMigrationAdoptsLegacyDatabase builds a database that only has the very
// first schema version, then lets Migrate adopt and upgrade it. Building the
// legacy schema from the migration file keeps this test independent of how many
// later migrations exist, so it does not need editing when one is added.
func TestMigrationAdoptsLegacyDatabase(t *testing.T) {
	databaseURL := os.Getenv("IM_TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("set IM_TEST_DATABASE_URL for integration tests")
	}
	ctx := t.Context()
	admin, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(admin.Close)
	schema := "im_legacy_" + model.NewID()
	if _, err = admin.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		cleanup, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if _, err := admin.Exec(cleanup, "DROP SCHEMA "+schema+" CASCADE"); err != nil {
			t.Error(err)
		}
	})
	migrations := filepath.Join("..", "..", "..", "database", "migrations")
	initial, err := os.ReadFile(filepath.Join(migrations, "000001_initial.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	config, err := pgxpool.ParseConfig(databaseURL)
	if err != nil {
		t.Fatal(err)
	}
	config.ConnConfig.RuntimeParams["search_path"] = schema
	db, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(db.Close)
	// The legacy database has the first schema but no migration ledger.
	if _, err = db.Exec(ctx, string(initial)); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec(ctx, `
		INSERT INTO users(id,email,name,password_hash) VALUES('legacy','legacy@test','legacy','unused');
		INSERT INTO conversations(id,kind,name,next_seq) VALUES('legacy-chat','group','legacy',0);
		INSERT INTO conversation_members(conversation_id,user_id) VALUES('legacy-chat','legacy')`); err != nil {
		t.Fatal(err)
	}
	store := &model.Store{DB: db}
	if err = store.Migrate(ctx, migrations); err != nil {
		t.Fatalf("adopting the legacy database failed: %v", err)
	}
	var count int
	if err = db.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatalf("legacy data lost: %d %v", count, err)
	}
	if err = db.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count < 9 {
		t.Fatalf("expected every migration to be recorded, got %d %v", count, err)
	}
	// The adopted database must now carry the newest columns.
	if err = db.QueryRow(ctx, `SELECT count(*) FROM information_schema.columns
		WHERE table_schema=current_schema() AND table_name='messages' AND column_name IN ('mentions','recalled_at','reply_to_id')`).Scan(&count); err != nil || count != 3 {
		t.Fatalf("upgraded schema is missing columns: %d %v", count, err)
	}
	// The legacy group must have been given an owner during the upgrade.
	var owner string
	if err = db.QueryRow(ctx, "SELECT COALESCE(created_by,'') FROM conversations WHERE id='legacy-chat'").Scan(&owner); err != nil || owner != "legacy" {
		t.Fatalf("legacy group did not receive an owner: %q %v", owner, err)
	}
	// Running Migrate again must be a no-op rather than an error.
	if err = store.Migrate(ctx, migrations); err != nil {
		t.Fatalf("re-running migrations failed: %v", err)
	}
}
