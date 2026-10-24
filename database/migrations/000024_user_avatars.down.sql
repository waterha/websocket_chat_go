DELETE FROM message_media WHERE conversation_id IS NULL;
ALTER TABLE users DROP COLUMN avatar_media_id;
ALTER TABLE message_media ALTER COLUMN conversation_id SET NOT NULL;
