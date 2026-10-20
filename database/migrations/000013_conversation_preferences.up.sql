-- Per-member conversation preferences: pin, mute and archive. These are private
-- to each member, so they live on the membership row rather than the
-- conversation, and they never affect what other members see.

ALTER TABLE conversation_members ADD COLUMN pinned_at timestamptz;
ALTER TABLE conversation_members ADD COLUMN muted boolean NOT NULL DEFAULT false;
ALTER TABLE conversation_members ADD COLUMN archived_at timestamptz;

-- The sidebar orders by pin first and then by recency, filtered by archive, so
-- the ordering keys are indexed per member.
CREATE INDEX conversation_members_prefs_idx ON conversation_members(user_id, archived_at, pinned_at DESC);
