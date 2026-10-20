-- Retention for published outbox rows and withdrawn attachments.
--
-- Two unbounded growth problems existed: every published outbox row stayed
-- forever (the payload holds an event copy plus a member snapshot), and a
-- recalled attachment kept its file on disk. Both are now cleaned by a worker,
-- but each needs a marker so the cleanup is measurable and can be reviewed.

-- Outbox rows are indexed by publication age so the retention worker can find
-- removable rows without scanning the whole table.
CREATE INDEX message_outbox_published_age_idx ON message_outbox(published_at) WHERE published_at IS NOT NULL;

-- A withdrawn attachment is marked removed once its bytes are deleted. The row
-- is kept so the message still resolves to a record of what was withdrawn, which
-- keeps history honest without retaining the content.
ALTER TABLE message_media ADD COLUMN bytes_removed_at timestamptz;
CREATE INDEX message_media_removable_idx ON message_media(created_at) WHERE bytes_removed_at IS NULL;
