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

RUN go build -o /app/comet ./services/comet

FROM alpine:latest
RUN apk add --no-cache netcat-openbsd
COPY --from=builder /app/comet /usr/local/bin/comet
EXPOSE 8081 9006
CMD ["comet"]
