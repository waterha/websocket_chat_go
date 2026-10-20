ALTER TABLE messages DROP CONSTRAINT messages_content_check;
ALTER TABLE messages ADD CONSTRAINT messages_content_check CHECK (
    (kind IN ('user', 'system') AND char_length(content) BETWEEN 1 AND 4000) OR
    (kind IN ('image', 'video') AND media_id IS NOT NULL AND char_length(content) <= 4000)
);

ALTER TABLE messages DROP CONSTRAINT messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check CHECK (kind IN ('user', 'system', 'image', 'video'));

ALTER TABLE message_media DROP CONSTRAINT message_media_media_type_check;
ALTER TABLE message_media ADD CONSTRAINT message_media_media_type_check CHECK (media_type IN ('image', 'video'));
