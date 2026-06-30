"""Ranking handler — periodically compute hot scores and maintain hot_posts ZSET.

hot_score = (likes×3 + comments×5) / (hours_since_post + 2)^1.5
"""
import logging
import time

logger = logging.getLogger(__name__)

HOT_POSTS_KEY = "hot_posts"
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


def run_ranking(*, redis, pg) -> None:
    """Compute hot scores for recent posts and refresh the hot_posts ZSET."""
    with pg.cursor() as cur:
        cur.execute(_SCORE_SQL, (RANKING_WINDOW_DAYS, TOP_N))
        rows = cur.fetchall()

    if not rows:
        logger.info("ranking: no posts found in last %d days", RANKING_WINDOW_DAYS)
        return

    # Atomic refresh: compute new scores first, then swap.
    tmp_key = f"{HOT_POSTS_KEY}:tmp"
    pipe = redis.pipeline()
    for post_id, score in rows:
        pipe.zadd(tmp_key, {str(post_id): float(score)})
    pipe.rename(tmp_key, HOT_POSTS_KEY)
    pipe.execute()

    logger.info("ranking: refreshed hot_posts with %d posts (top score=%.2f)", len(rows), rows[0][1])
