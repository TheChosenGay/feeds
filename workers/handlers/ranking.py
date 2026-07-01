"""Ranking handler — periodically compute hot scores, refresh hot_posts ZSET,
and pre-cache top posts into Redis so the timeline doesn't touch PostgreSQL.

hot_score = (likes×3 + comments×5) / (hours_since_post + 2)^1.5
"""
import json
import logging

logger = logging.getLogger(__name__)

HOT_POSTS_KEY = "hot_posts"
POST_CACHE_TTL = 600  # 10 minutes, matches Go postCacheTTL
RANKING_WINDOW_DAYS = 7
TOP_N = 1000

# language=PostgreSQL
_SCORE_SQL = """
WITH stats AS (
    SELECT
        p.id,
        EXTRACT(EPOCH FROM (NOW() - p.created_at)) / 3600.0 AS hours_ago,
        COALESCE(l.like_count, 0) AS likes,
        COALESCE(c.comment_count, 0) AS comments
    FROM feed.posts p
    LEFT JOIN (
        SELECT post_id, COUNT(*) AS like_count
        FROM interaction.post_likes
        GROUP BY post_id
    ) l ON l.post_id = p.id
    LEFT JOIN (
        SELECT post_id, COUNT(*) AS comment_count
        FROM interaction.post_comments
        GROUP BY post_id
    ) c ON c.post_id = p.id
    WHERE p.created_at > NOW() - INTERVAL '%s days'
)
SELECT
    id,
    (likes * 3.0 + comments * 5.0) / POWER(hours_ago + 2.0, 1.5) AS hot_score
FROM stats
ORDER BY hot_score DESC
LIMIT %s
"""

# language=PostgreSQL
_POST_SQL = """
SELECT id, author_id, blocks, created_at, updated_at
FROM feed.posts
WHERE id = ANY(%s)
"""


def run_ranking(*, redis, pg) -> None:
    """Compute hot scores, refresh ZSET, and pre-cache top posts in Redis."""
    with pg.cursor() as cur:
        cur.execute(_SCORE_SQL, (RANKING_WINDOW_DAYS, TOP_N))
        rows = cur.fetchall()

    if not rows:
        logger.info("ranking: no posts found in last %d days", RANKING_WINDOW_DAYS)
        return

    post_ids = [str(row[0]) for row in rows]

    # ── 1. Refresh hot_posts ZSET ──
    # Build in tmp key, then rename atomically.
    tmp = f"{HOT_POSTS_KEY}:tmp"
    pipe = redis.pipeline()
    for post_id, score in rows:
        pipe.zadd(tmp, {str(post_id): float(score)})
    pipe.execute()
    redis.rename(tmp, HOT_POSTS_KEY)

    # ── 2. Pre-cache top post content (only what's missing) ──
    # Check which posts are already cached.
    cache_keys = [f"post:{pid}" for pid in post_ids]
    cached = [pid for pid, exists in zip(post_ids, redis.mget(cache_keys)) if exists]

    missing = [pid for pid in post_ids if pid not in cached]
    if missing:
        _cache_posts(redis, pg, missing)
        logger.info(
            "ranking: %d posts cached, %d already in cache, %d skipped",
            len(missing), len(cached), 0,
        )
    else:
        logger.info("ranking: all %d posts already cached", len(cached))

    logger.info("ranking: hot_posts refreshed, top %d, best score=%.2f",
                len(rows), rows[0][1])


def _cache_posts(redis, pg, post_ids: list[str]) -> None:
    """Fetch post content from PostgreSQL and SET post:{id} in Redis."""
    with pg.cursor() as cur:
        cur.execute(_POST_SQL, (post_ids,))
        db_rows = cur.fetchall()

    # Build map: post_id → JSON matching GetFeedResp proto shape.
    posts: dict[str, dict] = {}
    for row in db_rows:
        pid, author_id, blocks_json, created_at, updated_at = row
        posts[str(pid)] = {
            "id": str(pid),
            "author_id": str(author_id),
            "blocks": blocks_json if isinstance(blocks_json, list) else json.loads(blocks_json),
            "created_at": int(created_at.timestamp()),
            "updated_at": int(updated_at.timestamp()),
        }

    pipe = redis.pipeline()
    for pid in post_ids:
        if pid in posts:
            pipe.set(f"post:{pid}", json.dumps(posts[pid]), ex=POST_CACHE_TTL)
    pipe.execute()
