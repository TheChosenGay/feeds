FROM golang:1.25.3-alpine AS builder
WORKDIR /app
COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/ ./proto/gen/
COPY services/comet/ ./services/comet/
COPY services/feed/ ./services/feed/
COPY services/gateway/ ./services/gateway/
COPY services/user/ ./services/user/
COPY services/interaction/ ./services/interaction/
COPY services/notify/ ./services/notify/
RUN go build -o /app/notify ./services/notify

FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/notify /usr/local/bin/notify
EXPOSE 9007
CMD ["notify"]
