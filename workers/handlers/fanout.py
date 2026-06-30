"""Fanout handler — on post.created, push to follower inboxes."""
import os
import logging

logger = logging.getLogger(__name__)

FANOUT_THRESHOLD = int(os.getenv("FANOUT_THRESHOLD", "1000"))


def handle_post_created(event: dict, *, redis, pg) -> None:
    """Process a post.created event: write to follower inboxes or author outbox."""
    post_id = event["post_id"]
    author_id = event["author_id"]
    created_at = event["created_at"]
    follower_count = event.get("follower_count", 0)

    if follower_count >= FANOUT_THRESHOLD:
        # big V: write to outbox only, readers will pull
        redis.zadd(f"outbox:{author_id}", {post_id: created_at})
        return

    # normal user: push to all followers' inboxes
    followers = _get_followers(pg, author_id)
    if not followers:
        return

    pipe = redis.pipeline()
    for follower_id in followers:
        pipe.zadd(f"inbox:{follower_id}", {post_id: created_at})
        pipe.zremrangebyrank(f"inbox:{follower_id}", 0, -1001)  # keep latest 1000
    pipe.execute()

    logger.info(
        "fanout: post=%s author=%s → %d inboxes", post_id, author_id, len(followers)
    )


def _get_followers(pg, user_id: str) -> list[str]:
    """Return all follower IDs for the given user from the user.follows table."""
    with pg.cursor() as cur:
        cur.execute(
            'SELECT follower_id FROM "user".follows WHERE followed_id = %s',
            (user_id,),
        )
        return [str(row[0]) for row in cur.fetchall()]
