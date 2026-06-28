CREATE SCHEMA IF NOT EXISTS interaction;

SET search_path TO interaction;

CREATE TABLE IF NOT EXISTS post_likes (
    user_id    UUID NOT NULL,
    post_id    UUID NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, post_id)
);

CREATE TABLE IF NOT EXISTS post_comments (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    post_id    UUID NOT NULL,
    content    TEXT NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_comments_post ON post_comments(post_id, created_at DESC);

CREATE TABLE IF NOT EXISTS post_bookmarks (
    user_id    UUID NOT NULL,
    post_id    UUID NOT NULL,
    created_at TIMESTAMPTZ DEFAULT now(),
    PRIMARY KEY (user_id, post_id)
);
