# Kafka 事件驱动架构

## 整体数据流

```
用户操作
  │
  ▼
┌──────────────┐     produce          ┌─────────┐     consume        ┌──────────────────┐
│  Go Services  │  ───────────────→    │  Kafka  │  ──────────────→   │  Python Workers   │
│               │                      │         │                     │                   │
│  feed-svc ────┤  post.created       │ Broker  │  post.created       │  fanout handler   │
│  interaction  │  post.liked          │         │  post.liked         │  → Redis inboxes  │
│  user-svc     │  post.commented      │         │  post.commented     │  → notify.Push()  │
└──────────────┘                      └─────────┘                     │                   │
                                                                       │  ranking cron      │
                                                                       │  → hot_posts ZSET  │
                                                                       └────────┬──────────┘
                                                                                │
                                                                         notify.Push()
                                                                                │
                                                                      ┌─────────┴─────────┐
                                                                      │   notify service   │
                                                                      │                    │
                                                                      │  1. IsOnline?      │
                                                                      │  2. DB insert      │
                                                                      │  3. live.PushUser  │
                                                                      └────────┬───────────┘
                                                                               │
                                                                      ┌────────┴────────┐
                                                                      │   live service   │
                                                                      │                  │
                                                                      │  PushToRoom()    │
                                                                      │   → WS frame     │
                                                                      └────────┬─────────┘
                                                                               │
                                                                         [0x00 0x03] + payload
                                                                               │
                                                                      ┌────────┴────────┐
                                                                      │   用户设备        │
                                                                      │   实时收到通知    │
                                                                      └─────────────────┘
```

## 通知链路详情

```
发帖/点赞/评论
  │
  ▼
Go Service (feed-svc / interaction-svc)
  │
  ├── DB write (同步)
  │
  └── Kafka produce (异步)
        │
        │  event = {
        │    "event":      "post.created",
        │    "post_id":    "xxx",
        │    "author_id":  "xxx",
        │    "created_at": 1234567890
        │  }
        │
        ▼
Kafka Broker
  │
  │  partition 按 key hash 分布
  │  consumer group 自动负载均衡
  │
  ▼
Python Consumer (fanout-worker)
  │
  ├── 分析事件类型
  │
  ├── fanout: 写到粉丝 Redis inbox
  │     redis.zadd("inbox:{follower_id}", {post_id: score})
  │
  └── notify.Push(author_id, "系统通知", "帖子发布成功")
        │
        ▼
notify service (:9007)
  │
  ├── 1. INSERT INTO notifications (...)  ← 持久化
  │
  ├── 2. live.IsOnline(user_id)           ← 查在线状态
  │
  └── 3. 在线? live.PushUser() : return    ← 实时投递
        │
        ▼
live service (:9006)
  │
  │  ConnManager.PushToRoom(userID, frame)
  │  → 找到该用户的所有 WS 连接
  │  → 逐个 WriteMessage(frame)
  │
  ▼
用户设备
  │
  │  收到二进制帧: [0x00 0x03] + JSON payload
  │  {
  │    "id":      1001,
  │    "type":    "like",
  │    "title":   "新点赞",
  │    "body":    "xxx 赞了你的帖子《今天天气真好》",
  │    "payload": {"post_id": "xxx"}
  │  }
  │
  └── 客户端解析 + 展示通知

  // 离线用户: 下次上线调 notify.ListNotifications() 拉取未读
```

## 延迟分析

| 环节 | 典型耗时 | 说明 |
|------|---------|------|
| Go → Kafka produce | <1ms | 同机房，`acks=1`，一个 RTT |
| Kafka → Consumer poll | <10ms | KafkaConsumer 长轮询，有消息立刻返回 |
| Handler 处理 | <50ms | Redis pipeline + PG 查询 |
| notify.Push gRPC | <5ms | 同主机 localhost |
| live.PushUser WS write | <1ms | 内存操作，write to socket |
| **总计** | **P50 < 100ms** | 感知上接近实时 |

**延迟尖刺**（P99）:
- 大 V 粉丝数 >1000 时 fanout 写 Redis 批量操作耗时长（~100-500ms）
- Consumer 有 lag 时排在队列后面
- GC pause（Go 或 Python）

## Kafka 配置要点

### Producer (Go)

```go
// kafka.Writer 关键配置
&kafka.Writer{
    Addr:         kafka.TCP("kafka:29092"),
    Balancer:     &kafka.Hash{},       // 按 key 哈希分区，同 post 事件有序
    BatchSize:    100,                 // 凑满 100 条或超时才发
    BatchTimeout: 10 * time.Millisecond,
    RequiredAcks: kafka.RequireOne,    // leader 确认即可
    Async:        false,               // 同步写，保证不丢
}
```

### Consumer (Python)

```python
# KafkaConsumer 关键配置
consumer = KafkaConsumer(
    *hd.topics,                           # ["post.created", "post.liked"]
    bootstrap_servers="kafka:29092",
    group_id="fanout-worker",             # consumer group，多实例自动分片
    auto_offset_reset="earliest",         # 新 group 从头消费
    enable_auto_commit=True,              # 自动提交 offset
    max_poll_records=500,                 # 单次拉取上限
    max_poll_interval_ms=300000,          # 5 分钟，处理大 V fanout 可能耗时
)
```

## Topic 设计

| Topic | Producer | Consumer | 说明 |
|-------|----------|----------|------|
| `post.created` | feed-svc | fanout handler | 新帖推送到粉丝 inbox |
| `post.liked` | interaction-svc | (待实现) | 通知帖主有人点赞 |
| `post.commented` | interaction-svc | (待实现) | 通知帖主有人评论 |

## 文件索引

| 组件 | 路径 |
|------|------|
| Kafka Producer 封装 | `pkg/kafka/` |
| Consumer 框架 | `workers/common/consumer.py` |
| Fanout handler | `workers/handlers/fanout.py` |
| Ranking cron | `workers/handlers/ranking.py` |
| Worker 入口 | `workers/fanout/main.py` |
| Docker Compose | `docker-compose.yml` → `fanout-worker` |
| notify 服务 | `services/notify/` |
| live 服务 | `services/live/` + `pkg/comet/` |
| E2E 测试 | `services/notify/e2e/main.go` |
