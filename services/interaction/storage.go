package main

import (
	"context"
	"fmt"
	"sync"

	"github.com/redis/go-redis/v9"
)

func likesKey(feedID string) string  { return "likes:" + feedID }
func likedByKey(feedID string) string { return "liked_by:" + feedID }

// cachedLikeStorage wraps LikeRepo with Redis counters.
//
// Write path:  Redis INCR + SADD  →  DB Insert (sync, non-fatal on failure)
// Read path:   Redis GET / SISMEMBER  →  fallback to DB
type cachedLikeStorage struct {
	inner *LikeRepo
	redis *redis.Client
}

func NewCachedLikeStorage(repo *LikeRepo, rdb *redis.Client) *cachedLikeStorage {
	return &cachedLikeStorage{inner: repo, redis: rdb}
}

// Like increments the Redis counter, adds user to the liked-by set, and persists to DB.
// Returns the new count. DB write failure is logged but does not fail the request —
// the counter is the source of truth and can be repaired from the DB.
func (s *cachedLikeStorage) Like(ctx context.Context, userID, feedID string) (int64, error) {
	pipe := s.redis.Pipeline()
	incr := pipe.Incr(ctx, likesKey(feedID))
	sadd := pipe.SAdd(ctx, likedByKey(feedID), userID)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("storage: like: %w", err)
	}

	// Persist to DB. Failure is non-fatal: Redis wins, repair via DB COUNT later.
	if err := s.inner.Insert(ctx, userID, feedID); err != nil {
		fmt.Printf("[cachedLikeStorage] db insert failed (non-fatal): %v\n", err)
	}

	return incr.Val(), sadd.Err()
}

// Unlike decrements the counter, removes user from the liked-by set, and deletes from DB.
func (s *cachedLikeStorage) Unlike(ctx context.Context, userID, feedID string) (int64, error) {
	pipe := s.redis.Pipeline()
	decr := pipe.Decr(ctx, likesKey(feedID))
	srem := pipe.SRem(ctx, likedByKey(feedID), userID)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("storage: unlike: %w", err)
	}

	count := decr.Val()
	if count < 0 {
		s.redis.Set(ctx, likesKey(feedID), 0, 0)
		count = 0
	}

	if err := s.inner.Delete(ctx, userID, feedID); err != nil {
		fmt.Printf("[cachedLikeStorage] db delete failed (non-fatal): %v\n", err)
	}

	return count, srem.Err()
}

// Count reads from Redis; falls back to DB if key is missing, then backfills.
func (s *cachedLikeStorage) Count(ctx context.Context, feedID string) (int64, error) {
	count, err := s.redis.Get(ctx, likesKey(feedID)).Int64()
	if err == nil {
		return count, nil
	}
	if err != redis.Nil {
		return 0, fmt.Errorf("storage: get likes count: %w", err)
	}

	// Cache miss — fallback to DB and backfill Redis
	dbCount, err := s.inner.Count(ctx, feedID)
	if err != nil {
		return 0, err
	}
	s.redis.Set(ctx, likesKey(feedID), dbCount, 0)
	return dbCount, nil
}

// BatchCount returns like counts for multiple feeds.
// All keys are fetched in a single Redis pipeline round-trip.
// Misses are resolved against the DB concurrently, then backfilled to Redis.
func (s *cachedLikeStorage) BatchCount(ctx context.Context, feedIDs []string) (map[string]int64, error) {
	if len(feedIDs) == 0 {
		return map[string]int64{}, nil
	}

	result := make(map[string]int64, len(feedIDs))

	// 1. Pipeline all GETs — one round-trip.
	//    Each pipe.Get() registers the command AND returns a *StringCmd handle.
	//    After pipe.Exec(), results are automatically populated into each handle.
	pipe := s.redis.Pipeline()
	cmds := make([]*redis.StringCmd, len(feedIDs))
	for i, id := range feedIDs {
		cmds[i] = pipe.Get(ctx, likesKey(id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("storage: batch count: %w", err)
	}

	// 2. Separate hits from misses.
	missed := make([]string, 0)
	for i, id := range feedIDs {
		count, err := cmds[i].Int64()
		if err == nil {
			result[id] = count
		} else if err == redis.Nil {
			missed = append(missed, id)
		} else {
			return nil, fmt.Errorf("storage: batch count %s: %w", id, err)
		}
	}

	if len(missed) == 0 {
		return result, nil
	}

	// 3. Concurrent DB fallback. Each goroutine writes to its own index — no lock needed.
	type missResult struct {
		id    string
		count int64
	}
	missResults := make([]missResult, len(missed))
	var wg sync.WaitGroup

	for i, id := range missed {
		wg.Add(1)
		go func(idx int, feedID string) {
			defer wg.Done()
			dbCount, err := s.inner.Count(ctx, feedID)
			if err != nil {
				fmt.Printf("[cachedLikeStorage] batch db count %s failed: %v\n", feedID, err)
				return
			}
			missResults[idx] = missResult{id: feedID, count: dbCount}
		}(i, id)
	}
	wg.Wait()

	// 4. Backfill Redis in a single pipeline.
	backfill := s.redis.Pipeline()
	for _, mr := range missResults {
		if mr.id == "" {
			continue // failed query, skip
		}
		result[mr.id] = mr.count
		backfill.Set(ctx, likesKey(mr.id), mr.count, 0)
	}
	if _, err := backfill.Exec(ctx); err != nil {
		fmt.Printf("[cachedLikeStorage] batch backfill redis: %v\n", err)
	}
	return result, nil
}

// IsLiked checks the Redis set first, falls back to DB.
func (s *cachedLikeStorage) IsLiked(ctx context.Context, userID, feedID string) (bool, error) {
	exists, err := s.redis.SIsMember(ctx, likedByKey(feedID), userID).Result()
	if err == nil {
		return exists, nil
	}
	if err != redis.Nil {
		return false, fmt.Errorf("storage: is liked: %w", err)
	}
	return s.inner.IsLiked(ctx, userID, feedID)
}
