DROP INDEX messages_mentions_idx;
DROP INDEX messages_reply_to_idx;

ALTER TABLE messages DROP COLUMN mentions;
ALTER TABLE messages DROP COLUMN recalled_at;
ALTER TABLE messages DROP COLUMN reply_to_id;
