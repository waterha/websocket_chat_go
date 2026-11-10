ALTER TABLE favorites
    ADD COLUMN tags text[] NOT NULL DEFAULT '{}',
    ADD COLUMN note text NOT NULL DEFAULT '',
    ADD CONSTRAINT favorite_tags_limit CHECK (cardinality(tags)<=10),
    ADD CONSTRAINT favorite_note_limit CHECK (char_length(note)<=2000);
