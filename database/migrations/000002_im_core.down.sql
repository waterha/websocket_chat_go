DROP TABLE IF EXISTS message_outbox, device_cursors;
ALTER TABLE conversation_members DROP CONSTRAINT IF EXISTS read_cursor_nonnegative;
ALTER TABLE conversation_members DROP COLUMN IF EXISTS read_at;
DROP INDEX IF EXISTS sessions_expiry_idx;
