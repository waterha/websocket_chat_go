-- A message can carry several images interleaved with text, in the order the
-- sender wrote them.
--
-- The previous shape allowed one attachment per message (messages.media_id), so a
-- note that explains two screenshots had to be split into several messages and the
-- layout could not match what the user typed. A message now also stores an ordered
-- list of blocks:
--
--   [{"type":"text","text":"看这张"},{"type":"image","media_id":"...", ...}, ...]
--
-- messages.content keeps the plain text (the text blocks joined by newlines), so
-- search, reply excerpts and notifications keep working without understanding
-- blocks. media_id stays for messages sent before this change and for the simple
-- one-attachment case.

ALTER TABLE messages ADD COLUMN blocks jsonb;

-- A message with blocks must hold a JSON array; the length bound stops a client
-- from posting an enormous document in one message.
ALTER TABLE messages ADD CONSTRAINT messages_blocks_shape CHECK (
    blocks IS NULL OR (jsonb_typeof(blocks) = 'array' AND jsonb_array_length(blocks) <= 20)
);
