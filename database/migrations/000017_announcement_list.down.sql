ALTER TABLE conversations ADD COLUMN announcement text NOT NULL DEFAULT '';
ALTER TABLE conversations ADD CONSTRAINT conversations_announcement_length CHECK (char_length(announcement) <= 2000);

-- Restore the single announcement from the most recent row, because the old
-- column could only hold one value.
UPDATE conversations c SET announcement = COALESCE((
    SELECT a.content FROM announcements a
    WHERE a.conversation_id = c.id
    ORDER BY a.created_at DESC, a.id DESC LIMIT 1
), '');

DROP INDEX announcements_conversation_idx;
DROP TABLE announcements;
