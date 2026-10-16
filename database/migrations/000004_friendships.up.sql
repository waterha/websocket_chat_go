CREATE TABLE friendships (
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    friend_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, friend_id),
    CHECK (user_id <> friend_id)
);
CREATE INDEX friendships_friend_idx ON friendships(friend_id);
