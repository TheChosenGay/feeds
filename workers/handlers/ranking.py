"""Ranking handler — periodically compute hot scores, refresh hot_posts ZSET,
and pre-cache top posts into Redis.

hot_score = (likes×3 + comments×5) / (hours_since_post + 2)^1.5

Counts are fetched from Redis (likes) and PostgreSQL (comments) — no
cross-schema JOINs, no full-table aggregation scans.
"""
import json
import logging
from datetime import datetime, timezone

logger = logging.getLogger(__name__)

HOT_POSTS_KEY = "hot_posts"
POST_CACHE_TTL = 600  # 10 minutes, matches Go postCacheTTL
RANKING_WINDOW_DAYS = 7
TOP_N = 1000


def run_ranking(*, redis, pg) -> None:
    """Compute hot scores, refresh ZSET, and pre-cache top posts in Redis."""
    # ── 1. Get recent posts (IDs + created_at only, no JOINs) ──
    with pg.cursor() as cur:
        cur.execute(
            "SELECT id, created_at FROM feed.posts "
            "WHERE created_at > NOW() - INTERVAL '%s days' "
            "ORDER BY created_at DESC",
            (RANKING_WINDOW_DAYS,),
        )
        posts = [(str(row[0]), row[1]) for row in cur.fetchall()]

    if not posts:
        logger.info("ranking: no posts in last %d days", RANKING_WINDOW_DAYS)
        return

    post_ids = [pid for pid, _ in posts]
    now = datetime.now(timezone.utc)

    # ── 2. Like counts from Redis (O(N) but all in-memory, sub-ms each) ──
    like_keys = [f"likes:{pid}" for pid in post_ids]
    like_vals = redis.mget(like_keys)
    likes = {
        pid: int(v) if v else 0
        for pid, v in zip(post_ids, like_vals)
    }

    # ── 3. Comment counts from PG (single indexed GROUP BY, no JOINs) ──
    with pg.cursor() as cur:
        cur.execute(
            "SELECT post_id, COUNT(*) FROM interaction.post_comments "
            "WHERE post_id = ANY(%s) GROUP BY post_id",
            (post_ids,),
        )
        comments = {str(row[0]): row[1] for row in cur.fetchall()}

    # ── 4. Compute scores ──
    scored = []
    for pid, created_at in posts:
        hours = (now - created_at.replace(tzinfo=timezone.utc)).total_seconds() / 3600.0
        l = likes.get(pid, 0)
        c = comments.get(pid, 0)
        score = (l * 3.0 + c * 5.0) / ((hours + 2.0) ** 1.5)
        scored.append((pid, score))

    scored.sort(key=lambda x: x[1], reverse=True)
    top = scored[:TOP_N]

    # ── 5. Refresh hot_posts ZSET ──
    tmp = f"{HOT_POSTS_KEY}:tmp"
    pipe = redis.pipeline()
    for pid, score in top:
        pipe.zadd(tmp, {pid: score})
    pipe.execute()
    redis.rename(tmp, HOT_POSTS_KEY)

    # ── 6. Pre-cache top post content ──
    top_ids = [pid for pid, _ in top]
    cache_keys = [f"post:{pid}" for pid in top_ids]
    cached_mask = redis.mget(cache_keys)
    missing = [pid for pid, exists in zip(top_ids, cached_mask) if not exists]

    if missing:
        _cache_posts(redis, pg, missing)
        logger.info("ranking: %d cached, %d already cached, total %d posts",
                     len(missing), len(top_ids) - len(missing), len(posts))
    else:
        logger.info("ranking: all %d top posts already cached (scanned %d total)",
                     len(top_ids), len(posts))

    logger.info("ranking: hot_posts refreshed, top=%d, best_score=%.2f",
                len(top), top[0][1])


def _cache_posts(redis, pg, post_ids: list[str]) -> None:
    """Fetch post content from PG and SET post:{id} in Redis (GetFeedResp JSON shape)."""
    with pg.cursor() as cur:
        cur.execute(
            "SELECT id, author_id, blocks, created_at, updated_at "
            "FROM feed.posts WHERE id = ANY(%s)",
            (post_ids,),
        )
        db_rows = cur.fetchall()

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
