"""Fanout worker entry point."""
import logging

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(levelname)s %(message)s",
)

from common.connections import new_redis, new_postgres  # noqa: E402
from common.consumer import HandlerDef, run_workers  # noqa: E402
from handlers.fanout import handle_post_created  # noqa: E402

if __name__ == "__main__":
    run_workers(
        [
            HandlerDef(
                group_id="fanout-worker-v3",
                topics=["post.created"],
                handler=handle_post_created,
            ),
        ],
        redis=new_redis(),
        pg=new_postgres(),
    )
