package storage

import (
	"context"
	"fmt"

	"github.com/redis/go-redis/v9"
)

// NewRedisClient creates a Redis client. Caller is responsible for defer client.Close().
func NewRedisClient(addr, password string, db int) (*redis.Client, error) {
	rdb := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
		DB:       db,
	})
	if err := rdb.Ping(context.Background()).Err(); err != nil {
		rdb.Close()
		return nil, fmt.Errorf("storage: connect redis: %w", err)
	}
	return rdb, nil
}

// RedisHealth reports whether Redis is reachable.
func RedisHealth(rdb *redis.Client) error {
	return rdb.Ping(context.Background()).Err()
}
