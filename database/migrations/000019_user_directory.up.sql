CREATE TABLE user_profiles (
    user_id text PRIMARY KEY REFERENCES users(id) ON DELETE CASCADE,
    department text NOT NULL DEFAULT '',
    job_title text NOT NULL DEFAULT '',
    updated_at timestamptz NOT NULL DEFAULT now()
);
