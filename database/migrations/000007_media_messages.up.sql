CREATE TABLE message_media (
    id text PRIMARY KEY,
    owner_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    file_name text NOT NULL CHECK (char_length(file_name) BETWEEN 1 AND 255),
    media_type text NOT NULL CHECK (media_type IN ('image', 'video')),
    content_type text NOT NULL CHECK (char_length(content_type) BETWEEN 1 AND 128),
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    storage_key text NOT NULL UNIQUE CHECK (char_length(storage_key) BETWEEN 1 AND 255),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX message_media_conversation_idx ON message_media(conversation_id, created_at DESC);

ALTER TABLE messages ADD COLUMN media_id text REFERENCES message_media(id);
CREATE UNIQUE INDEX messages_media_once_idx ON messages(media_id) WHERE media_id IS NOT NULL;

-- Constraint names are dropped defensively: a database upgraded from the
-- earliest version still carries the original names, and replaying this file
-- after a manual downgrade must not fail on an already removed constraint.
ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check CHECK (kind IN ('user', 'system', 'image', 'video'));

ALTER TABLE messages DROP CONSTRAINT IF EXISTS messages_content_check;
ALTER TABLE messages ADD CONSTRAINT messages_content_check CHECK (
    (kind IN ('user', 'system') AND char_length(content) BETWEEN 1 AND 4000) OR
    (kind IN ('image', 'video') AND media_id IS NOT NULL AND char_length(content) <= 4000)
);
