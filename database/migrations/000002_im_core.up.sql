ALTER TABLE conversation_members ADD COLUMN read_at timestamptz;
ALTER TABLE conversation_members ADD CONSTRAINT read_cursor_nonnegative CHECK (last_read_seq >= 0);

CREATE TABLE device_cursors (
    user_id text NOT NULL,
    conversation_id text NOT NULL,
    device_id text NOT NULL CHECK (char_length(device_id) BETWEEN 1 AND 100),
    received_seq bigint NOT NULL DEFAULT 0 CHECK (received_seq >= 0),
    updated_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, conversation_id, device_id),
    FOREIGN KEY (conversation_id, user_id) REFERENCES conversation_members(conversation_id, user_id) ON DELETE CASCADE
);

CREATE TABLE message_outbox (
    id bigserial PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    recipients text[] NOT NULL,
    payload jsonb NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    published_at timestamptz
);
CREATE INDEX message_outbox_pending_idx ON message_outbox(id) WHERE published_at IS NULL;
CREATE INDEX sessions_expiry_idx ON sessions(expires_at);
