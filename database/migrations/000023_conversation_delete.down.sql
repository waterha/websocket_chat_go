DROP INDEX conversation_members_deleted_idx;
ALTER TABLE conversation_members DROP COLUMN deleted_at;
