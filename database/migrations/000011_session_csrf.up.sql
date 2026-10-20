-- Session-bound CSRF token and password change support.

-- Existing sessions predate this column and therefore have no token; the
-- middleware treats a missing token as "re-login required" rather than trusting
-- a request that cannot be verified.
ALTER TABLE sessions ADD COLUMN csrf_token text;

-- Password changes rotate every other session for that user, which makes
-- "sign out everywhere" fall out of the same mechanism.
ALTER TABLE sessions ADD COLUMN created_at timestamptz NOT NULL DEFAULT now();
