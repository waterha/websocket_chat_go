-- Contact controls: the remark a user gives a friend (visible only to them,
-- like conversation preferences) and the personal block list. Blocking is
-- one-directional: the blocker stops seeing requests and direct messages from
-- the blocked user, who is told they were blocked only where necessary.

ALTER TABLE friendships ADD COLUMN remark text NOT NULL DEFAULT '';

CREATE TABLE user_blocks (
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    blocked_user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (user_id, blocked_user_id),
    CHECK (user_id <> blocked_user_id)
);
CREATE INDEX user_blocks_blocked_idx ON user_blocks(blocked_user_id);
