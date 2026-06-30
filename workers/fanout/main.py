"""Worker entry point — Kafka consumers + cron jobs."""
import logging

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(name)s] %(levelname)s %(message)s",
)

from common.connections import new_redis, new_postgres  # noqa: E402
from common.consumer import HandlerDef, CronDef, run_workers  # noqa: E402
from handlers.fanout import handle_post_created  # noqa: E402
from handlers.ranking import run_ranking  # noqa: E402

if __name__ == "__main__":
    run_workers(
        handlers=[
            HandlerDef(
                group_id="fanout-worker",
                topics=["post.created"],
                handler=handle_post_created,
            ),
        ],
        crons=[
            CronDef(
                name="ranking",
                interval=300,  # every 5 minutes
                handler=run_ranking,
            ),
        ],
        redis=new_redis(),
        pg=new_postgres(),
    )
