"""Worker factory — Kafka consumers + cron-style scheduled tasks."""
import json
import logging
import os
import threading
import time
from dataclasses import dataclass, field
from collections.abc import Callable

from kafka import KafkaConsumer

logger = logging.getLogger(__name__)

Handler = Callable[..., None]


# ── Kafka consumer ──────────────────────────────────────────────


@dataclass
class HandlerDef:
    """Register a Kafka consumer."""

    group_id: str
    topics: list[str]
    handler: Handler


# ── Cron / scheduled task ───────────────────────────────────────


@dataclass
class CronDef:
    """Register a periodic task (not driven by Kafka events)."""

    name: str
    interval: int  # seconds between runs
    handler: Handler
    run_on_start: bool = True


# ── Runner ──────────────────────────────────────────────────────


def run_workers(
    handlers: list[HandlerDef] | None = None,
    crons: list[CronDef] | None = None,
    *,
    redis,
    pg,
    notify,
) -> None:
    """Start every registered worker in its own thread.

    HandlerDef → Kafka consumer (one per group, poll loop).
    CronDef    → simple sleep-loop thread, calls handler every *interval* seconds.

    All threads share the same Redis / PostgreSQL / notify connections.
    """
    brokers = os.getenv("KAFKA_BROKERS", "localhost:9092")
    threads: list[threading.Thread] = []

    # ── Kafka consumers ──
    for hd in handlers or []:
        t = threading.Thread(
            target=_run_consumer,
            args=(hd, brokers, redis, pg, notify),
            name=hd.group_id,
            daemon=True,
        )
        threads.append(t)

    # ── Cron jobs ──
    for cd in crons or []:
        t = threading.Thread(
            target=_run_cron,
            args=(cd, redis, pg, notify),
            name=f"cron:{cd.name}",
            daemon=True,
        )
        threads.append(t)

    logger.info("starting %d workers (%d consumers + %d crons)...",
                len(threads), len(handlers or []), len(crons or []))

    for t in threads:
        t.start()

    for t in threads:
        t.join()


def _run_consumer(hd: HandlerDef, brokers: str, redis, pg, notify) -> None:
    logger.info("consumer starting: group=%s topics=%s", hd.group_id, hd.topics)

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
                hd.handler(event, redis=redis, pg=pg, notify=notify)
            except Exception:
                logger.exception(
                    "handler error: group=%s topic=%s offset=%s",
                    hd.group_id, msg.topic, msg.offset,
                )
    finally:
        consumer.close()


def _run_cron(cd: CronDef, redis, pg, notify) -> None:
    logger.info("cron starting: name=%s interval=%ds", cd.name, cd.interval)

    if cd.run_on_start:
        try:
            cd.handler(redis=redis, pg=pg, notify=notify)
        except Exception:
            logger.exception("cron error (startup): name=%s", cd.name)

    while True:
        time.sleep(cd.interval)
        try:
            cd.handler(redis=redis, pg=pg, notify=notify)
        except Exception:
            logger.exception("cron error: name=%s", cd.name)
