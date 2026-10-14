package api

import (
	"testing"
)

func TestMigrationAdoptsLegacyDatabase(t *testing.T) {
	services := fixture(t, false)
	ctx := t.Context()
	_, err := services.Store.DB.Exec(ctx, `DROP TABLE message_outbox,device_cursors,schema_migrations; ALTER TABLE conversation_members DROP COLUMN read_at; ALTER TABLE conversation_members DROP CONSTRAINT read_cursor_nonnegative; DROP INDEX sessions_expiry_idx`)
	if err != nil {
		t.Fatal(err)
	}
	account(t, services, "legacy")
	if err := services.Store.Migrate(ctx, services.Config.MigrationsDir); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM users").Scan(&count); err != nil || count != 1 {
		t.Fatal("legacy data lost", err)
	}
	if err := services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 2 {
		t.Fatal("bad migration versions", err)
	}
}
