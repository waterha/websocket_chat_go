-- User avatars reuse the media storage. An avatar is a media row without a
-- conversation, downloadable by every authenticated user through the regular
-- media endpoint.

ALTER TABLE message_media ALTER COLUMN conversation_id DROP NOT NULL;
ALTER TABLE users ADD COLUMN avatar_media_id text REFERENCES message_media(id) ON DELETE SET NULL;
