# 开发进展

## 架构设计决策

### 1. 帖子 vs 互动 — 拆分为独立服务

帖子 (Post) 和互动 (Interaction) **必须拆分**，读写特征完全不同：

| | Post Service | Interaction Service |
|---|---|---|
| **职责** | 帖子 CRUD、内容存储 | 点赞、评论、收藏 |
| **读写比** | 写一次，读万次 | 写频繁（热门帖子万级点赞） |
| **扩展瓶颈** | 读压力 | 写压力 |
| **交互方式** | — | 同步 gRPC 查询，异步 Kafka 发布事件 |

**服务交互原则**：读走同步 (gRPC)，写发事件 (Kafka)。

```
┌──────────┐  同步查询评论列表    ┌──────────────┐
│  Gateway │ ────gRPC──────────→ │ Post Service  │
│          │ ←───gRPC─────────── │ (帖子CRUD)    │
│          │                     └──────────────┘
│          │  同步查询点赞数      ┌──────────────────┐
│          │ ────gRPC──────────→ │ Interaction Svc   │
│          │ ←───gRPC─────────── │ (点赞/评论/收藏)   │
└──────────┘                     └──────────────────┘
```

### 2. 消息 Push 策略 — Kafka 事件驱动 + Worker 分发

三层递进：

```
用户动作 (发帖/点赞/评论/关注)
    │
    ▼
业务服务发布 Kafka 事件
    │
    ├── "post.created"   → fanout worker        → 写扩散到粉丝 inbox (已有)
    ├── "post.liked"     → notification worker   → 创建通知记录
    ├── "post.commented" → notification worker   → 创建通知记录
    └── "user.followed"  → notification worker   → 创建通知记录
                                │
                                ▼
                         ┌──────────────┐
                         │ 通知记录落库   │  (PostgreSQL)
                         │ 在线用户推送   │  (WebSocket, Gateway 维护)
                         │ 离线用户静默   │  (下次打开时拉取)
                         └──────────────┘
```

- **Kafka 解耦** — 业务服务只管发事件，不管谁消费
- **Worker 消费** — fanout 写 inbox，notification 写通知表
- **实时推送** — Gateway 维护 WebSocket；离线用户下次拉取

### 3. 用户服务独立 + 跨服务处理

用户信息**必须独立服务**。跨服务场景三种策略：

| 策略 | 场景 | 说明 |
|---|---|---|
| **Gateway 聚合** | 读路径（推荐） | Gateway 并行调多个服务，拼装结果返回客户端 |
| **数据冗余** | 高频读（后期优化） | Kafka 消费 `user.profile.updated`，下游缓存 `author_avatar`，前期不要用 |
| **gRPC 链式调用** | 写路径校验 | Post Service → User Service 校验用户是否存在 |

**原则**：读走 Gateway 聚合，写走 gRPC 校验 + Kafka 异步。

### 4. 性能分析方案

| 阶段 | 工具 | 用途 |
|---|---|---|
| **本地开发** | Go pprof（`net/http/pprof`） | CPU/内存/协程火焰图 |
| **单元级** | `go test -bench` | 函数级基准测试 |
| **接口级** | k6 / Vegeta | HTTP 压测，验证 QPS 和延迟 |
| **上线后** | OpenTelemetry + Jaeger/Tempo | 全链路追踪，定位瓶颈环节 |

### 5. 监控 — Grafana LGTM 全家桶

在基础设施中增加：

| 组件 | 端口 | 用途 |
|---|---|---|
| **Prometheus** | 9090 | 指标采集（QPS、延迟、错误率） |
| **Loki** | 3100 | 日志聚合 |
| **Tempo** | 3200 | 链路追踪 |
| **Grafana** | 3000 | 统一可视化面板 |

Go 服务接入：
- `promhttp.Handler()` 暴露 `/metrics`（Prometheus 指标）
- OpenTelemetry SDK 自动注入 trace context（gRPC/HTTP 链路追踪）
- `log/slog` 结构化日志 → Loki 采集

备选：Datadog（贵）、SigNoz（开源但不够成熟）。

---

## 服务规划

```
feeds/
├── proto/                    # Protobuf 服务定义
│   ├── user.proto            # 用户服务（已有）
│   ├── post.proto            # 帖子服务（待建）
│   └── interaction.proto     # 互动服务（待建）
├── pkg/                      # Go 共享库
│   └── config/               # 配置加载
├── services/
│   ├── gateway/              # API Gateway (REST → gRPC)
│   ├── user/                 # 用户/关注 (gRPC)
│   ├── post/                 # 帖子 CRUD (gRPC)
│   ├── feed/                 # Feed 流组装 (gRPC)
│   └── interaction/          # 点赞/评论/收藏 (gRPC, 待建)
├── workers/                  # Python 异步 Workers
│   ├── fanout/               # 写扩散
│   ├── notification/         # 通知分发 (待建)
│   ├── ranking/              # 互动值计算
│   └── recommend/            # 推荐排序
└── docker-compose.yml
```

---

## 阶段规划

### Phase 1: 项目骨架与基础设施
- [x] 确定技术方案
- [x] 初始化项目结构 (Monorepo + Go workspace)
- [x] Docker Compose (PostgreSQL + Redis + Kafka)
- [x] Protobuf 服务定义（user.proto）
- [x] 共享库 (config)

### Phase 2: 用户服务
- [ ] 数据模型 (users 表)
- [ ] 注册/登录 (JWT)
- [ ] 用户资料 CRUD
- [ ] 关注/取关

### Phase 3: 发帖服务
- [ ] proto: post.proto + interaction.proto
- [ ] 数据模型 (posts / comments / likes 表)
- [ ] 发帖 (文字/图片/视频/链接)
- [ ] 评论
- [ ] 点赞/收藏
- [ ] Kafka 事件发布

### Phase 4: Feed 流服务
- [ ] 混合 Feed 模型实现
- [ ] inbox/outbox Redis 结构
- [ ] Feed 流组装与分页
- [ ] 写扩散 Worker (完善 fanout)

### Phase 5: 通知 & 搜索 & 推荐
- [ ] notification worker (Kafka → 通知表 + WebSocket)
- [ ] PostgreSQL 全文搜索
- [ ] 互动值计算 Worker
- [ ] 推荐排序

### Phase 6: 监控 & 可观测性
- [ ] Prometheus + Grafana + Loki + Tempo
- [ ] OpenTelemetry 链路追踪
- [ ] Go pprof 接入

### Phase 7: 生产部署
- [ ] 腾讯云资源规划
- [ ] K8s 部署配置
- [ ] 监控与告警

---

## 当前进度

**当前阶段**: Phase 2 - 用户服务

### 2026-06-27
- 确定架构设计决策（服务拆分、消息推送、跨服务调用、性能监控方案）
- 更新项目规划文档

### 2026-05-30
- 完成技术方案选型
- 创建项目文档
