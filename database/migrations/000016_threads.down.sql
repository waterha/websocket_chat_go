DROP INDEX messages_thread_root_lookup_idx;
DROP INDEX messages_thread_idx;

ALTER TABLE messages DROP COLUMN thread_root_id;
