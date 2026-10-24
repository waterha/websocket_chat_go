-- Emoji reactions on messages. The (message, user, emoji) key makes a toggle
-- idempotent, and reactions disappear with their message.

CREATE TABLE message_reactions (
    message_id text NOT NULL REFERENCES messages(id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    emoji text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (message_id, user_id, emoji)
);
CREATE INDEX message_reactions_message_idx ON message_reactions(message_id);
