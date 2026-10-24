DROP TABLE conversation_join_requests;
ALTER TABLE conversations DROP COLUMN dismissed_at;
ALTER TABLE conversations DROP COLUMN join_approval;
ALTER TABLE conversations DROP COLUMN all_muted;
ALTER TABLE conversation_members DROP COLUMN muted_until;
ALTER TABLE conversation_members DROP COLUMN alias;
