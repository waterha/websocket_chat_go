DROP INDEX conversation_members_prefs_idx;

ALTER TABLE conversation_members ADD COLUMN archived_at timestamptz;
CREATE INDEX conversation_members_prefs_idx ON conversation_members(user_id, archived_at, pinned_at DESC);
