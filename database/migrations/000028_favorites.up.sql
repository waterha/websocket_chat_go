CREATE TABLE favorites (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    message_id text NOT NULL,
    conversation_id text NOT NULL,
    message jsonb NOT NULL,
    sender_name text NOT NULL,
    sender_avatar text NOT NULL DEFAULT '',
    conversation_name text NOT NULL,
    conversation_kind text NOT NULL,
    is_own boolean NOT NULL,
    saved_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE (user_id, message_id)
);
CREATE INDEX favorites_page_idx ON favorites(user_id, saved_at DESC, id DESC);

CREATE TABLE favorite_assets (
    favorite_id text NOT NULL REFERENCES favorites(id) ON DELETE CASCADE,
    media_id text NOT NULL,
    file_name text NOT NULL,
    media_type text NOT NULL,
    content_type text NOT NULL,
    byte_size bigint NOT NULL CHECK (byte_size > 0),
    storage_key text NOT NULL UNIQUE,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (favorite_id, media_id)
);
CREATE TABLE favorite_asset_cleanup (
    storage_key text PRIMARY KEY,
    ready_at timestamptz NOT NULL DEFAULT now(),
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE FUNCTION queue_favorite_asset_cleanup() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
    INSERT INTO favorite_asset_cleanup(storage_key) VALUES (OLD.storage_key) ON CONFLICT DO NOTHING;
    RETURN OLD;
END;
$$;
CREATE TRIGGER favorite_asset_cleanup_on_delete AFTER DELETE ON favorite_assets
    FOR EACH ROW EXECUTE FUNCTION queue_favorite_asset_cleanup();
