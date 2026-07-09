"""Shared connections for all workers — created once, reused across handlers."""
import os

import grpc
import psycopg
import redis
from notify import notify_pb2_grpc


def new_redis() -> redis.Redis:
    addr = os.getenv("REDIS_ADDR", "localhost:6379")
    return redis.Redis.from_url(f"redis://{addr}/0")


def new_postgres():
    dsn = os.getenv(
        "POSTGRES_DSN",
        "host=localhost port=5432 user=feeds password=feeds_dev dbname=feeds sslmode=disable",
    )
    return psycopg.connect(dsn)


def new_notify_stub():
    """Create a gRPC stub for the notify service."""
    addr = os.getenv("NOTIFY_ADDR", "localhost:9007")
    channel = grpc.insecure_channel(addr)
    return notify_pb2_grpc.NotifyServiceStub(channel)
