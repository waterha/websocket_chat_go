-- Reply quoting, @mentions and message recall.

ALTER TABLE messages ADD COLUMN reply_to_id text REFERENCES messages(id);
ALTER TABLE messages ADD COLUMN recalled_at timestamptz;
ALTER TABLE messages ADD COLUMN mentions text[] NOT NULL DEFAULT '{}';

CREATE INDEX messages_reply_to_idx ON messages(reply_to_id) WHERE reply_to_id IS NOT NULL;
CREATE INDEX messages_mentions_idx ON messages USING gin(mentions);
