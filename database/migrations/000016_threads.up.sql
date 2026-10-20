-- Threads: a message that is replied to becomes the root of a conversation
-- window holding every reply to it.
--
-- reply_to_id already records the direct parent for quoting, but it cannot
-- answer "show me every reply to this message" without a recursive walk, and it
-- nests replies inside replies. A stored thread root keeps threads flat: every
-- reply points at one root, so a window is a single indexed lookup and a reply
-- to a reply joins the same window as the original.

ALTER TABLE messages ADD COLUMN thread_root_id text REFERENCES messages(id);

-- The window query filters on the root and orders by conversation sequence, so
-- the index covers both the lookup and the ordering.
CREATE INDEX messages_thread_idx ON messages(thread_root_id, seq) WHERE thread_root_id IS NOT NULL;

-- The main chat view asks "how many replies does this message have" for the
-- messages it is already showing, which is the same lookup.
CREATE INDEX messages_thread_root_lookup_idx ON messages(thread_root_id) WHERE thread_root_id IS NOT NULL;
