-- Archiving gave a conversation a second lifecycle without changing what any
-- member could actually see or do, so it is removed. Pin and mute remain: they
-- are cheap, and both change something the user notices.

DROP INDEX conversation_members_prefs_idx;

ALTER TABLE conversation_members DROP COLUMN archived_at;

-- The list still sorts pinned conversations first, so the index keeps only the
-- two remaining preference keys.
CREATE INDEX conversation_members_prefs_idx ON conversation_members(user_id, pinned_at DESC);
