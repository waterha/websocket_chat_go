-- Preserve existing audio bytes and messages as downloadable files on rollback.
UPDATE messages SET kind = 'file' WHERE kind = 'audio';
UPDATE message_media SET media_type = 'file' WHERE media_type = 'audio';

ALTER TABLE messages DROP CONSTRAINT messages_content_check;
ALTER TABLE messages ADD CONSTRAINT messages_content_check CHECK (
    (kind IN ('user', 'system') AND char_length(content) BETWEEN 1 AND 4000) OR
    (kind IN ('image', 'video', 'file') AND media_id IS NOT NULL AND char_length(content) <= 4000) OR
    (blocks IS NOT NULL AND jsonb_array_length(blocks) > 0 AND char_length(content) <= 4000)
);

ALTER TABLE messages DROP CONSTRAINT messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check CHECK (kind IN ('user', 'system', 'image', 'video', 'file'));

ALTER TABLE message_media DROP CONSTRAINT message_media_media_type_check;
ALTER TABLE message_media ADD CONSTRAINT message_media_media_type_check CHECK (media_type IN ('image', 'video', 'file'));
