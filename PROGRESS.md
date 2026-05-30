# 开发进展

## 阶段规划

### Phase 1: 项目骨架与基础设施
- [x] 确定技术方案
- [ ] 初始化项目结构 (Monorepo + Go workspace)
- [ ] Docker Compose (PostgreSQL + Redis + Kafka)
- [ ] Protobuf 服务定义
- [ ] 共享库 (数据库连接 / Redis / Kafka / 中间件)

### Phase 2: 用户服务
- [ ] 数据模型 (users 表)
- [ ] 注册/登录 (JWT)
- [ ] 用户资料 CRUD
- [ ] 关注/取关

### Phase 3: 发帖服务
- [ ] 数据模型 (posts / comments / likes 表)
- [ ] 发帖 (文字/图片/视频/链接)
- [ ] 评论
- [ ] 点赞/收藏
- [ ] Kafka 事件发布

### Phase 4: Feed 流服务
- [ ] 混合 Feed 模型实现
- [ ] inbox/outbox Redis 结构
- [ ] Feed 流组装与分页
- [ ] 写扩散 Worker (Python)

### Phase 5: 搜索与推荐
- [ ] PostgreSQL 全文搜索
- [ ] 互动值计算 Worker
- [ ] 推荐排序

### Phase 6: 生产部署
- [ ] 腾讯云资源规划
- [ ] K8s 部署配置
- [ ] 监控与告警

---

## 当前进度

**当前阶段**: Phase 1 - 项目骨架与基础设施

### 2026-05-30
- 完成技术方案选型
- 创建项目文档
