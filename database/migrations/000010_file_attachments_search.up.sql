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

-- Global search matches with `strpos(lower(content), lower(query))`, which is a
-- function call rather than a LIKE operator, so a pg_trgm GIN index cannot
-- accelerate it. No index is created here; the query is bounded by the caller's
-- membership, a recency ordering and a row limit. Adding a real search index
-- (for example a tsvector column or an external search service) is future work.
