-- Restore the content rule the blocks branch replaced, then drop the column.
ALTER TABLE messages DROP CONSTRAINT messages_content_check;
ALTER TABLE messages ADD CONSTRAINT messages_content_check CHECK (
    (kind IN ('user', 'system') AND char_length(content) BETWEEN 1 AND 4000) OR
    (kind IN ('image', 'video', 'file') AND media_id IS NOT NULL AND char_length(content) <= 4000)
);

ALTER TABLE messages DROP CONSTRAINT messages_blocks_shape;
ALTER TABLE messages DROP COLUMN blocks;
