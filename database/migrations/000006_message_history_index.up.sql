-- The unique (conversation_id, seq) B-tree supports both forward sync and
-- backward history scans when conversation_id is fixed. Keep that constraint.
-- IF EXISTS also supports adopted legacy databases without this extra index.
DROP INDEX IF EXISTS messages_history_idx;
