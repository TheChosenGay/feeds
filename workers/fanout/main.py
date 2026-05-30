"""Fan-out worker: consumes post.created events and writes to follower inboxes."""
import os
import json
import redis
from confluent_kafka import Consumer, KafkaError

KAFKA_BROKERS = os.getenv("KAFKA_BROKERS", "localhost:9092")
REDIS_ADDR = os.getenv("REDIS_ADDR", "localhost:6379")
FANOUT_THRESHOLD = int(os.getenv("FANOUT_THRESHOLD", "1000"))


def main():
    r = redis.Redis.from_url(f"redis://{REDIS_ADDR}/0")

    consumer = Consumer({
        "bootstrap.servers": KAFKA_BROKERS,
        "group.id": "fanout-worker",
        "auto.offset.reset": "earliest",
    })
    consumer.subscribe(["post.created"])

    print("fanout worker started, consuming post.created...")

    try:
        while True:
            msg = consumer.poll(1.0)
            if msg is None:
                continue
            if msg.error():
                if msg.error().code() == KafkaError._PARTITION_EOF:
                    continue
                print(f"consumer error: {msg.error()}")
                continue

            event = json.loads(msg.value().decode("utf-8"))
            handle_post_created(r, event)
    finally:
        consumer.close()


def handle_post_created(r: redis.Redis, event: dict):
    post_id = event["post_id"]
    author_id = event["author_id"]
    created_at = event["created_at"]
    follower_count = event.get("follower_count", 0)

    if follower_count >= FANOUT_THRESHOLD:
        # big V: write to outbox only, readers will pull
        r.zadd(f"outbox:{author_id}", {post_id: created_at})
        return

    # normal user: push to all followers' inboxes
    followers = get_followers(author_id)
    pipe = r.pipeline()
    for follower_id in followers:
        pipe.zadd(f"inbox:{follower_id}", {post_id: created_at})
        pipe.zremrangebyrank(f"inbox:{follower_id}", 0, -1001)  # keep latest 1000
    pipe.execute()


def get_followers(user_id: str) -> list[str]:
    # TODO: query from PostgreSQL or Redis cache
    return []


if __name__ == "__main__":
    main()
