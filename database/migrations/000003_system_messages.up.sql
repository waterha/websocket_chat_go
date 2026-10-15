ALTER TABLE messages ADD COLUMN kind text NOT NULL DEFAULT 'user' CHECK (kind IN ('user', 'system'));
