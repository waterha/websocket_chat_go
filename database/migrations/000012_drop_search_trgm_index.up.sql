-- Drop the pg_trgm index that migration 000010 originally created. It could not
-- accelerate global search, because matching uses strpos() rather than a LIKE
-- operator, and creating it required the pg_trgm extension to be visible in the
-- migration's search_path. Databases that already applied the original 000010
-- still carry the index; this migration removes it so every database converges.
DROP INDEX IF EXISTS messages_content_trgm_idx;
