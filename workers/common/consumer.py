"""Consumer factory — create Kafka consumers with shared connections."""
import os
import json
import logging
import threading
from dataclasses import dataclass
from collections.abc import Callable

from kafka import KafkaConsumer

logger = logging.getLogger(__name__)

Handler = Callable[..., None]


@dataclass
class HandlerDef:
    """A handler definition: consumer group, topics, and handler function."""

    group_id: str
    topics: list[str]
    handler: Handler


def run_workers(handlers: list[HandlerDef], *, redis, pg) -> None:
    """Start one Kafka consumer per handler, each in its own thread.

    All consumers share the same Redis and PostgreSQL connections. Each
    HandlerDef gets its own consumer group so offsets are managed
    independently.

    Usage::

        run_workers(
            [
                HandlerDef(
                    group_id="fanout-worker",
                    topics=["post.created"],
                    handler=handle_post_created,
                ),
                HandlerDef(
                    group_id="notification-worker",
                    topics=["feed.liked", "feed.commented"],
                    handler=handle_notification,
                ),
            ],
            redis=new_redis(),
            pg=new_postgres(),
        )
    """

    brokers = os.getenv("KAFKA_BROKERS", "localhost:9092")
    threads: list[threading.Thread] = []

    for hd in handlers:
        t = threading.Thread(
            target=_run_one,
            args=(hd, brokers, redis, pg),
            name=hd.group_id,
            daemon=True,
        )
        threads.append(t)

    logger.info("starting %d workers...", len(handlers))
    for t in threads:
        t.start()

    # Block until all threads finish (they won't, unless Kafka disconnects).
    for t in threads:
        t.join()


def _run_one(hd: HandlerDef, brokers: str, redis, pg) -> None:
    """Run a single consumer loop in a dedicated thread."""
    logger.info("worker starting: group=%s topics=%s", hd.group_id, hd.topics)

    consumer = KafkaConsumer(
        *hd.topics,
        bootstrap_servers=brokers,
        group_id=hd.group_id,
        auto_offset_reset="earliest",
    )

    try:
        for msg in consumer:
            try:
                event = json.loads(msg.value.decode("utf-8"))
                hd.handler(event, redis=redis, pg=pg)
            except Exception:
                logger.exception(
                    "handler error: group=%s topic=%s offset=%s",
                    hd.group_id,
                    msg.topic,
                    msg.offset,
                )
    finally:
        consumer.close()
