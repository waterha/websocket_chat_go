-- Group governance: an explicit owner, optional admins, an announcement, and a
-- record of who created the conversation so existing groups can be backfilled.

ALTER TABLE conversations ADD COLUMN announcement text NOT NULL DEFAULT '';
ALTER TABLE conversations ADD COLUMN created_by text REFERENCES users(id);
ALTER TABLE conversations ADD CONSTRAINT conversations_announcement_length CHECK (char_length(announcement) <= 2000);

ALTER TABLE conversation_members ADD COLUMN role text NOT NULL DEFAULT 'member';
ALTER TABLE conversation_members ADD CONSTRAINT conversation_members_role CHECK (role IN ('owner', 'admin', 'member'));

-- Backfill the creator for existing groups: the earliest message sender is the
-- best available evidence, falling back to the lexicographically first member
-- so every legacy group ends up with exactly one owner.
UPDATE conversations c SET created_by = COALESCE(
    (SELECT m.sender_id FROM messages m WHERE m.conversation_id = c.id ORDER BY m.seq LIMIT 1),
    (SELECT cm.user_id FROM conversation_members cm WHERE cm.conversation_id = c.id ORDER BY cm.user_id LIMIT 1)
) WHERE c.kind = 'group';

UPDATE conversation_members cm SET role = 'owner'
FROM conversations c
WHERE c.id = cm.conversation_id AND c.kind = 'group' AND c.created_by = cm.user_id;

-- At most one owner per conversation; direct conversations have none.
CREATE UNIQUE INDEX conversation_members_owner_idx ON conversation_members(conversation_id) WHERE role = 'owner';

-- Removing a member must not delete their history. The original
-- messages(conversation_id, sender_id) foreign key required the sender to still
-- be a member, which made it impossible to remove anyone who had ever posted.
-- Sending already checks current membership in the application, so history is
-- kept and the sender may be a former member.
ALTER TABLE messages DROP CONSTRAINT messages_conversation_id_sender_id_fkey;
