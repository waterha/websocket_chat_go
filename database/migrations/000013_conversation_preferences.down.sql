DROP INDEX conversation_members_prefs_idx;

ALTER TABLE conversation_members DROP COLUMN archived_at;
ALTER TABLE conversation_members DROP COLUMN muted;
ALTER TABLE conversation_members DROP COLUMN pinned_at;
