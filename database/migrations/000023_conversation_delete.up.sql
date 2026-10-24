-- Deleting a conversation is private: the member keeps their membership and
-- history but the conversation disappears from their sidebar until the next
-- message clears the marker again.

ALTER TABLE conversation_members ADD COLUMN deleted_at timestamptz;
CREATE INDEX conversation_members_deleted_idx ON conversation_members(user_id, deleted_at);
