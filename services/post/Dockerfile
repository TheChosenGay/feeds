FROM golang:1.25.3-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/post/ ./proto/gen/post/
COPY services/post/ ./services/post/

RUN go build -o /app/feed-service ./services/post

FROM alpine:latest
COPY --from=builder /app/feed-service /usr/local/bin/feed-service
EXPOSE 9001
CMD ["feed-service"]
