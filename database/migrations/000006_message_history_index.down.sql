CREATE INDEX messages_history_idx ON messages(conversation_id, seq DESC);
