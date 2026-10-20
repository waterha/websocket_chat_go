DROP INDEX message_media_removable_idx;
ALTER TABLE message_media DROP COLUMN bytes_removed_at;

DROP INDEX message_outbox_published_age_idx;
