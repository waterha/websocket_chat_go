package api

import (
	"testing"
)

func TestMigrationAdoptsLegacyDatabase(t *testing.T) {
	services := fixture(t, false)
	ctx := t.Context()
	// Remove everything the later migrations add so the adoption path can be
	// replayed. The order matters: the rewritten content constraint references
	// both columns, and messages.media_id references the media table.
	_, err := services.Store.DB.Exec(ctx, `DROP INDEX conversation_members_owner_idx;
		DROP TABLE friend_requests,friendships,message_outbox,device_cursors,schema_migrations;
		ALTER TABLE messages DROP CONSTRAINT messages_content_check;
		ALTER TABLE messages DROP COLUMN media_id;
		ALTER TABLE messages DROP COLUMN kind;
		DROP TABLE message_media;
		ALTER TABLE conversation_members DROP CONSTRAINT conversation_members_role;
		ALTER TABLE conversation_members DROP COLUMN role;
		ALTER TABLE conversation_members DROP COLUMN read_at;
		ALTER TABLE conversation_members DROP CONSTRAINT read_cursor_nonnegative;
		ALTER TABLE conversations DROP CONSTRAINT conversations_announcement_length;
		ALTER TABLE conversations DROP COLUMN created_by;
		ALTER TABLE conversations DROP COLUMN announcement;
		DROP INDEX sessions_expiry_idx`)
	if err != nil {
		t.Fatal(err)
	}
	// The legacy schema still required a message sender to be a current member;
	// restoring that constraint proves migration 000008 copes with it.
	if _, err = services.Store.DB.Exec(ctx, `ALTER TABLE messages ADD CONSTRAINT messages_conversation_id_sender_id_fkey FOREIGN KEY (conversation_id, sender_id) REFERENCES conversation_members(conversation_id, user_id)`); err != nil {
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
	if err := services.Store.DB.QueryRow(ctx, "SELECT count(*) FROM schema_migrations").Scan(&count); err != nil || count != 8 {
		t.Fatal("bad migration versions", err)
	}
}
