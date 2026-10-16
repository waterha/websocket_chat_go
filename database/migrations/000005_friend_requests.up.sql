CREATE TABLE friend_requests (
    id text PRIMARY KEY,
    requester_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    recipient_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message text NOT NULL DEFAULT '',
    status text NOT NULL DEFAULT 'pending' CHECK (status IN ('pending', 'accepted', 'rejected')),
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now(),
    CHECK (requester_id <> recipient_id),
    CHECK (char_length(message) <= 500)
);
CREATE UNIQUE INDEX friend_requests_pending_pair_idx ON friend_requests(requester_id, recipient_id) WHERE status = 'pending';
CREATE INDEX friend_requests_recipient_status_idx ON friend_requests(recipient_id, status, updated_at DESC);
CREATE INDEX friend_requests_requester_status_idx ON friend_requests(requester_id, status, updated_at DESC);
