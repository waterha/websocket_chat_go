DROP INDEX messages_media_once_idx;

ALTER TABLE messages DROP CONSTRAINT messages_content_check;
ALTER TABLE messages ADD CONSTRAINT messages_content_check CHECK (char_length(content) BETWEEN 1 AND 4000);

ALTER TABLE messages DROP CONSTRAINT messages_kind_check;
ALTER TABLE messages ADD CONSTRAINT messages_kind_check CHECK (kind IN ('user', 'system'));

ALTER TABLE messages DROP COLUMN media_id;

DROP TABLE message_media;
