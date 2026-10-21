-- Group announcements become a collection rather than a single text column.
--
-- A group previously carried exactly one announcement in
-- conversations.announcement, which cannot express "this week's notice plus the
-- pinned rule" and cannot delete one notice without clearing everything. Each
-- announcement is now its own row with its own author and timestamps, so owners
-- and admins can create, edit and delete them individually while members read the
-- whole list.

CREATE TABLE announcements (
    id text PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    author_id text NOT NULL REFERENCES users(id),
    content text NOT NULL CHECK (char_length(content) BETWEEN 1 AND 2000),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);

-- The panel lists a conversation's announcements newest first.
CREATE INDEX announcements_conversation_idx ON announcements(conversation_id, created_at DESC);

-- Carry any existing single announcement across so nothing is lost. The author
-- is the group creator, which is the only identity the old column could imply.
INSERT INTO announcements(id,conversation_id,author_id,content)
SELECT md5(random()::text || c.id), c.id, c.created_by, c.announcement
FROM conversations c
WHERE c.kind = 'group' AND c.created_by IS NOT NULL AND length(trim(c.announcement)) > 0;

ALTER TABLE conversations DROP COLUMN announcement;
