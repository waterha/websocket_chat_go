-- Group governance: per-member alias (group card) and timed mute, group-wide
-- mute and join approval settings, the join request flow, and dismissal. A
-- dismissed group keeps its history readable but refuses new messages.

ALTER TABLE conversation_members ADD COLUMN alias text NOT NULL DEFAULT '';
ALTER TABLE conversation_members ADD COLUMN muted_until timestamptz;

ALTER TABLE conversations ADD COLUMN all_muted boolean NOT NULL DEFAULT false;
ALTER TABLE conversations ADD COLUMN join_approval boolean NOT NULL DEFAULT false;
ALTER TABLE conversations ADD COLUMN dismissed_at timestamptz;

CREATE TABLE conversation_join_requests (
    id text PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (char_length(message) <= 500)
);
-- One pending request per user and conversation; decided rows keep history.
CREATE UNIQUE INDEX conversation_join_requests_pending_idx ON conversation_join_requests(conversation_id, user_id) WHERE status = 'pending';
CREATE INDEX conversation_join_requests_conversation_idx ON conversation_join_requests(conversation_id, status, updated_at DESC);
