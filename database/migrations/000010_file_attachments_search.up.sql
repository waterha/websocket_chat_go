-- General file attachments and full text search across a user's conversations.

-- Files reuse the existing media table; only the kind and size limits differ.
ALTER TABLE message_media DROP CONSTRAINT message_media_media_type_check;
ALTER TABLE message_media ADD CONSTRAINT message_media_media_type_check CHECK (media_type IN ('image', 'video', 'file'));

ALTER TABLE messages DROP CONSTRAINT messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check CHECK (kind IN ('user', 'system', 'image', 'video', 'file'));

ALTER TABLE messages DROP CONSTRAINT messages_content_check;
ALTER TABLE messages ADD CONSTRAINT messages_content_check CHECK (
    (kind IN ('user', 'system') AND char_length(content) BETWEEN 1 AND 4000) OR
    (kind IN ('image', 'video', 'file') AND media_id IS NOT NULL AND char_length(content) <= 4000)
);

-- Global search filters by membership and orders by recency, so it needs an
-- index on the message body plus the existing per-conversation history index.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX messages_content_trgm_idx ON messages USING gin(lower(content) gin_trgm_ops);
