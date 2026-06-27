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
RUN go build -o /app/gateway ./services/gateway

# 运行阶段
FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/gateway /usr/local/bin/gateway
EXPOSE 8080
CMD ["gateway"]
