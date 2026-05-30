FROM golang:1.25.3-alpine AS builder

WORKDIR /app

# 复制 workspace 配置
COPY go.work go.work.sum ./

# 复制 go.work 中所有模块
COPY pkg/ ./pkg/
COPY proto/gen/user/ ./proto/gen/user/
COPY services/feed/ ./services/feed/
COPY services/gateway/ ./services/gateway/
COPY services/post/ ./services/post/
COPY services/user/ ./services/user/

# 编译
RUN go build -o /app/user-service ./services/user

# 运行阶段
FROM alpine:latest
COPY --from=builder /app/user-service /usr/local/bin/user-service
EXPOSE 9003
CMD ["user-service"]
