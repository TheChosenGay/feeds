FROM golang:1.25.3-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/ ./proto/gen/
COPY services/feed/ ./services/feed/
COPY services/gateway/ ./services/gateway/
COPY services/user/ ./services/user/
COPY services/interaction/ ./services/interaction/

RUN go build -o /app/interaction-service ./services/interaction

FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/interaction-service /usr/local/bin/interaction-service
EXPOSE 9005
CMD ["interaction-service"]
