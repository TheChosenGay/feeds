# Feeds

社区 Feed 流系统，支持发帖、评论、点赞、关注、搜索和智能推荐。

## 架构

- **微服务 Monorepo**：Go 服务 + Python 异步 Workers
- **服务间通信**：内部 gRPC，对外 REST
- **Feed 模型**：混合模式（Push + Pull，按粉丝数阈值切换）

## 技术栈

| 组件 | 技术选型 |
|------|---------|
| API Gateway | Go (gin + grpc-go) |
| 业务服务 | Go (PostSvc / FeedSvc / UserSvc) |
| 异步 Workers | Python (写扩散 / 排序 / 推荐) |
| 数据库 | PostgreSQL |
| 缓存 | Redis |
| 消息队列 | Kafka |
| 搜索 | PostgreSQL full-text (后期 ES) |
| 容器化 | Docker Compose (开发) / 腾讯云 TKE (生产) |

## 项目结构

```
feeds/
├── proto/               # Protobuf 服务定义
├── pkg/                 # Go 共享库
├── services/
│   ├── gateway/         # API Gateway
│   ├── post/            # 发帖/评论/点赞
│   ├── feed/            # Feed 流组装
│   └── user/            # 用户/关注
├── workers/             # Python 异步 Workers
│   ├── fanout/          # 写扩散
│   ├── ranking/         # 互动值计算
│   └── recommend/       # 推荐
├── deployments/         # 部署配置
├── migrations/          # 数据库迁移
└── docker-compose.yml
```

## 开发

```bash
# 启动基础设施
docker compose up -d postgres redis kafka

# 启动 Go 服务
go run ./services/gateway

# 启动 Python Worker
python -m workers.fanout
```

## Feed 流设计

混合模式：
- 粉丝数 < 1000：发帖时 Push 写入所有粉丝 inbox
- 粉丝数 >= 1000（大V）：读取时 Pull 合并大V的 outbox
- Redis ZSET 存储 inbox/outbox，score = 时间戳
