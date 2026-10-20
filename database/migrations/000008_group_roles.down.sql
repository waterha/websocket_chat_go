ALTER TABLE messages ADD CONSTRAINT messages_conversation_id_sender_id_fkey
    FOREIGN KEY (conversation_id, sender_id) REFERENCES conversation_members(conversation_id, user_id);

DROP INDEX conversation_members_owner_idx;

ALTER TABLE conversation_members DROP CONSTRAINT conversation_members_role;
ALTER TABLE conversation_members DROP COLUMN role;

ALTER TABLE conversations DROP CONSTRAINT conversations_announcement_length;
ALTER TABLE conversations DROP COLUMN created_by;
ALTER TABLE conversations DROP COLUMN announcement;
