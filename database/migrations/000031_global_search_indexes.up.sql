-- Literal Unicode substring search, including one/two-character Chinese terms,
-- emoji and punctuation. No extension or language-specific tokenizer is needed.
CREATE FUNCTION message_search_grams(body text) RETURNS text[]
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    WITH characters AS (
        SELECT ch,lead(ch,1) OVER w AS next1,lead(ch,2) OVER w AS next2
        FROM regexp_split_to_table(lower(body),'') WITH ORDINALITY AS parts(ch,pos)
        WINDOW w AS (ORDER BY pos)
    )
    SELECT COALESCE(array_agg(DISTINCT gram), ARRAY[]::text[])
    FROM characters CROSS JOIN LATERAL (VALUES (ch),(ch||next1),(ch||next1||next2)) terms(gram)
    WHERE gram IS NOT NULL
$$;

CREATE FUNCTION message_search_query_grams(body text) RETURNS text[]
LANGUAGE sql IMMUTABLE STRICT PARALLEL SAFE AS $$
    SELECT COALESCE(array_agg(DISTINCT substring(value FROM pos FOR width)), ARRAY[]::text[])
    FROM (SELECT lower(body) AS value, least(3, char_length(lower(body))) AS width) normalized
    CROSS JOIN LATERAL generate_series(1, char_length(value)-width+1) positions(pos)
    WHERE width>0
$$;

CREATE INDEX messages_global_search_grams_idx ON messages USING gin (message_search_grams(content)) WHERE recalled_at IS NULL;
CREATE INDEX messages_global_search_recent_idx ON messages(created_at DESC,id DESC) WHERE recalled_at IS NULL;
CREATE INDEX messages_global_search_conversation_idx ON messages(conversation_id,created_at DESC,id DESC) WHERE recalled_at IS NULL;
CREATE INDEX conversation_members_search_user_idx ON conversation_members(user_id,conversation_id);

-- Seed expression statistics for existing messages; autovacuum refreshes them
-- as the table grows. Statistics affect the route only, never correctness.
ANALYZE messages;
