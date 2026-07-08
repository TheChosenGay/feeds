FROM golang:1.25.3-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/ ./proto/gen/
COPY services/live/ ./services/live/
COPY services/feed/ ./services/feed/
COPY services/gateway/ ./services/gateway/
COPY services/user/ ./services/user/
COPY services/interaction/ ./services/interaction/
COPY services/notify/ ./services/notify/

RUN go build -o /app/live ./services/live

FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/live /usr/local/bin/live
EXPOSE 8081 9006
CMD ["live"]
