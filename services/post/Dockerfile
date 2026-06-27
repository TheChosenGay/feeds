FROM golang:1.25.3-alpine AS builder

WORKDIR /app

COPY go.work go.work.sum ./
COPY pkg/ ./pkg/
COPY proto/gen/post/ ./proto/gen/post/
COPY services/post/ ./services/post/

RUN go build -o /app/post-service ./services/post

FROM alpine:latest
COPY --from=builder /app/post-service /usr/local/bin/post-service
EXPOSE 9001
CMD ["post-service"]
