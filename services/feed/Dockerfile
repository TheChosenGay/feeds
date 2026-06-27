FROM golang:1.25.3-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/ ./proto/gen/
COPY services/feed/ ./services/feed/
COPY services/gateway/ ./services/gateway/
COPY services/user/ ./services/user/

RUN go build -o /app/feed-service ./services/feed

FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/feed-service /usr/local/bin/feed-service
EXPOSE 9001
CMD ["feed-service"]
