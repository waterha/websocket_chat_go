CREATE TABLE users (
    id text PRIMARY KEY,
    email text NOT NULL UNIQUE,
    name text NOT NULL,
    password_hash text NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT users_name_length CHECK (char_length(name) BETWEEN 1 AND 64)
);

CREATE TABLE sessions (
    token_hash text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_user_id_idx ON sessions(user_id);

CREATE TABLE conversations (
    id text PRIMARY KEY,
    name text NOT NULL DEFAULT '',
    kind text NOT NULL CHECK (kind IN ('direct', 'group')),
    direct_key text UNIQUE,
    next_seq bigint NOT NULL DEFAULT 0,
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT conversations_group_name CHECK (kind = 'direct' OR char_length(name) BETWEEN 1 AND 100)
);

CREATE TABLE conversation_members (
    conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    last_read_seq bigint NOT NULL DEFAULT 0,
    PRIMARY KEY (conversation_id, user_id)
);
CREATE INDEX conversation_members_user_idx ON conversation_members(user_id);

CREATE TABLE messages (
    id text PRIMARY KEY,
    conversation_id text NOT NULL REFERENCES conversations(id) ON DELETE CASCADE,
    sender_id text NOT NULL REFERENCES users(id),
    client_msg_id text NOT NULL,
    seq bigint NOT NULL,
    content text NOT NULL CHECK (char_length(content) BETWEEN 1 AND 4000),
    created_at timestamptz NOT NULL DEFAULT now(),
    UNIQUE (sender_id, client_msg_id),
    UNIQUE (conversation_id, seq),
    FOREIGN KEY (conversation_id, sender_id) REFERENCES conversation_members(conversation_id, user_id)
);
CREATE INDEX messages_history_idx ON messages(conversation_id, seq DESC);
