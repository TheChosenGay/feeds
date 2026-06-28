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
- **WebSocket** — Gateway 维护长连接池，在线用户即时推送，离线用户下次拉取

### 3. WebSocket 通知通道

Gateway 是唯一和客户端直接通信的服务，WebSocket 放在此处：

```
notification worker
    │
    ├── 写 notifications 表（持久化，离线用户下次拉取）
    ├── 查 Gateway：user 在线吗？
    │       │
    │       ├── 在线 → WebSocket 即时推送
    │       └── 离线 → 跳过
    │
    ▼
Gateway（WebSocket 连接池）
    user_123 ←→ conn_a
    user_456 ←→ conn_b
```

通知表：
```sql
CREATE TABLE notifications (
    id         UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id    UUID NOT NULL,
    type       VARCHAR(32) NOT NULL,  -- like / comment / follow
    actor_id   UUID,
    post_id    UUID,
    message    TEXT,
    read       BOOLEAN DEFAULT false,
    created_at TIMESTAMPTZ DEFAULT now()
);
```

### 6. 服务间依赖 — 零代码依赖，只信任数据

Interaction Service 不需要 import User 或 Post 的 protobuf：

```
Gateway（持有 JWT，验证身份）
    │
    ├── gRPC → Post Service（验证帖子存在）
    ├── gRPC → Interaction Service（写操作）
    └── 结果返回客户端
```

| | User Service | Post Service | Interaction Service |
|---|---|---|---|
| **知道什么** | 用户名、密码、头像 | 帖子内容、媒体 | user_id + post_id + 动作 |
| **依赖谁** | 无 | User（作者字段存在性） | **无** |
| **被谁调用** | Gateway | Gateway | Gateway |

- Interaction 只存 `user_id` / `post_id`，由 Gateway 先验证存在性
- 外键用 `ON DELETE CASCADE`：用户/帖子被删，互动自动清理
- Kafka 事件携带足够的冗余数据，消费者不需要回查源服务

### 9. 媒体上传 & COS 分发

上传独立于帖子，Gateway 提供通用上传入口：

```
发帖流程（一次请求）：
                              ③ 直传 COS（秒传，CDN 就近）
客户端 ──────────────────────────────────────→ COS/S3
  │                                                │
  │ ① 选图，本地取得 width/height/size/duration       │
  │ ② GET /upload/token → 后端返回预签名 URL         │
  │                                                │
  │ ④ 组装 blocks（带元数据）：                       │
  │   [{"type":"image","url":"cos://...",            │
  │     "width":1920,"height":1080}]                │
  │                                                │
  └──────────────────────────→ POST /posts ──→ Post Service
                               （只传 1-2 KB JSON）
```

加载流程（一次 API 请求，图片走 CDN）：
```
Feed 列表 → 返回帖子 JSON（含 width/height/duration）→ 客户端占位
           → <img src="cos://..."> 浏览器/App 从 CDN 加载（不经过后台）
```

COS 优势：
- CDN 就近分发，不占用服务器带宽
- 图片处理 URL 参数：`?imageMogr2/thumbnail/200x` 前端按需变尺寸
- 视频首帧：`?ci-process=snapshot&time=1`
- 通用的上传 Token 接口，头像/帖子/评论都能复用

Block 元数据（客户端上传前获取）：
| 字段 | 来源 | 作用 |
|---|---|---|
| `width / height` | 客户端选图时取得 | 页面占位，防止加载抖动 |
| `duration` | 客户端选视频时取得 | 显示时长 |
| `size` | 客户端选文件时取得 | 体积提示 |
| `cover_url` | 客户端截图或 COS 自动生成 | 视频封面 |

### 8. 推荐系统 — 分层 Worker

**ranking worker**：持续计算热度分
```
hot_score = (likes×3 + comments×5 + shares×10) / (hours_since_post + 2)^1.5
```

**recommend worker**：
- 协同过滤 + 内容召回 + 冷启动曝光
- 结果写入 `rec:{user_id}` ZSET
- Feed 组装时混排：关注时间线 + 推荐候选 + 热门

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
│   ├── feed/                 # 帖子 CRUD + Feed 流组装 (gRPC)
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
- [x] 数据模型 + Repository 层
- [x] 注册/登录 (bcrypt + JWT)
- [x] 迁移管理 (golang-migrate + go:embed)
- [ ] 关注/取关
- [ ] proto 代码生成 (待 buf 安装)

### Phase 3: 发帖服务 & 互动服务
- [x] proto: post.proto（Block 模型：图文视频混排 + 元数据）
- [x] 数据模型：posts 表（JSONB blocks），PostRepository CRUD + 分页
- [x] 媒体上传：Gateway 内置 `GET /upload/token`（COS 预签名 URL）
- [ ] proto: interaction.proto
- [ ] Interaction 服务（点赞/评论/收藏）
- [ ] Gateway 对 Post 的 HTTP 路由（POST /posts 等）
- [ ] Kafka 事件发布（post.created）

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

### 10. 帖子服务 & 互动服务 —— 读写分离

帖子（读多写少）和互动（写多读少）属于两种完全不同的负载模型，分开的目的不只是"按功能拆"，而是**按可靠性要求 & 扩展策略拆**——让两者可以采用完全不同的缓存/异步/降级方案。

#### 帖子服务 — 读瓶颈

帖子是 **WORM（Write Once, Read Many）** 模型。减压策略：

```
┌────────────────────────────────────────────────────────┐
│ 1. Redis 缓存热门帖子                                   │
│    key: post:{id}  value: 帖子 JSON                     │
│    TTL 1h，LRU 淘汰。预计 90% 读请求不碰数据库。         │
│                                                        │
│ 2. CDN 卸载媒体流量                                     │
│    图片/视频走 COS CDN，服务器只返回 2-3KB JSON。         │
│    媒体带宽成本归 CDN，服务端零压力。                     │
│                                                        │
│ 3. PostgreSQL 读副本                                    │
│    主库写，从库读。查询可无限水平扩展。                    │
│                                                        │
│ 4. 游标分页 + pageSize 上限（≤50）                       │
│    禁止深分页，减少扫描行数。                             │
└────────────────────────────────────────────────────────┘
```

#### 互动服务 — 写瓶颈

热门帖子可能每秒数百次点赞/评论。减压策略：

```
┌────────────────────────────────────────────────────────┐
│ 1. Redis 计数器代替 SQL COUNT                           │
│    INCR post:likes:{post_id} 每次 +1，O(1)。             │
│    计数器不准时从 DB 回源修正。                          │
│                                                        │
│ 2. Kafka 异步化写入                                     │
│    点赞 → 写 Redis + 发 Kafka 事件 → 立即返回。           │
│    消费者异步落库，用户体验不等待 DB 写入。              │
│                                                        │
│ 3. 批量 INSERT                                          │
│    消费者攒到 N 条后批量写入，减少连接开销。              │
│                                                        │
│ 4. 可降级                                               │
│    互动服务故障时点赞数暂时不准/延迟，核心浏览不受影响。   │
│    帖子服务故障则必须立即修，二者 SLO 完全不同。          │
└────────────────────────────────────────────────────────┘
```

#### 拆分后各服务的可靠性边界

| | 帖子服务 | 互动服务 |
|---|---|---|
| **读写模式** | 写一次，读万次 | 写极度频繁 |
| **瓶颈在哪** | 数据库读 | 数据库写 |
| **缓存策略** | 缓存帖子内容（大对象） | 缓存计数器（小数值） |
| **扩展方式** | 加读副本 | 异步写 + 分片 |
| **宕机影响** | 用户看不到内容，需立即修 | 点赞数暂时不准，可降级 |
| **一致性要求** | 强一致（内容不能丢） | 最终一致（计数差几个无所谓） |

如果揉在一起就会被强迫用最保守策略（强一致、同步写），无法对互动做降级和异步。

---

## 当前进度

**当前阶段**: Phase 3 - 发帖服务 & 互动服务

### 2026-06-28
- 确定帖子/互动服务读写分离方案：Post 读瓶颈（Redis 缓存 + CDN + 读副本），Interaction 写瓶颈（Redis 计数器 + Kafka 异步 + 批量写入）
- 确定 Fanout 性能优化方向：分批 pipeline 或统一 Pull 模式
- 实现互动服务：proto 定义（Like/Unlike/Comment/Bookmark，RPC 去 Post 化命名）、迁移（3 表）、repo（3 个 Repository）、gRPC 服务（端口 9005）
- Gateway 集成：9 条互动路由（POST/DELETE/GET /posts/{id}/like 等）
- Docker：Dockerfile + docker-compose 添加 interaction-service
- 下一步：关注/取关 → Kafka 事件打通 → Feed 流组装

### 2026-06-27
- 完成 pkg/storage（PG + MySQL + Redis 连接 + golang-migrate + embed）
- 完成 Phase 2 用户服务：model / repo / bcrypt / JWT / 迁移
- 确定：Interaction 服务零依赖、WebSocket 推送、多级缓存、推荐架构
- 开始 Phase 3：Post + Interaction 服务开发

### 2026-05-30
- 完成技术方案选型
- 创建项目文档
