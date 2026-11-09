CREATE TABLE stickers (
    id text PRIMARY KEY,
    user_id text NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    kind text NOT NULL CHECK (kind IN ('image','emoji')),
    source_key text NOT NULL,
    text text NOT NULL DEFAULT '',
    file_name text NOT NULL DEFAULT '',
    content_type text NOT NULL DEFAULT '',
    byte_size bigint NOT NULL DEFAULT 0,
    storage_key text UNIQUE,
    created_at timestamptz NOT NULL DEFAULT clock_timestamp(),
    UNIQUE(user_id,kind,source_key),
    CHECK ((kind='emoji' AND text<>'' AND storage_key IS NULL AND byte_size=0) OR (kind='image' AND text='' AND storage_key IS NOT NULL AND byte_size>0 AND byte_size<=10485760))
);
CREATE INDEX stickers_user_idx ON stickers(user_id,created_at DESC,id DESC);
CREATE TRIGGER sticker_asset_cleanup_on_delete AFTER DELETE ON stickers
    FOR EACH ROW WHEN (OLD.storage_key IS NOT NULL) EXECUTE FUNCTION queue_favorite_asset_cleanup();
