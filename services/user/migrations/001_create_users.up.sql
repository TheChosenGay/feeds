CREATE SCHEMA IF NOT EXISTS "user";

SET search_path TO "user";

CREATE TABLE IF NOT EXISTS users (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    username   VARCHAR(64) UNIQUE NOT NULL,
    password   VARCHAR(256) NOT NULL,
    avatar_url TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT now(),
    updated_at TIMESTAMPTZ DEFAULT now()
);

CREATE INDEX IF NOT EXISTS idx_users_username ON users(username);
