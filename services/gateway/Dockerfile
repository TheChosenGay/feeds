FROM golang:1.25.3-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/ ./proto/gen/
COPY services/feed/ ./services/feed/
COPY services/gateway/ ./services/gateway/
COPY services/user/ ./services/user/

RUN go build -o /app/gateway ./services/gateway

FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/gateway /usr/local/bin/gateway
EXPOSE 8080
CMD ["gateway"]
