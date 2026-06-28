package main

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/redis/go-redis/v9"
)

// likeTTL keeps cold like data from filling Redis indefinitely.
const likeTTL = 30 * 24 * time.Hour

// rebuildTTL is a short TTL for partial liked_by rebuilds. If a mid-rebuild
// batch fails, the incomplete set expires quickly instead of lingering.
const rebuildTTL = 30 * time.Minute

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

// Like adds a like. Checks membership first to avoid unnecessary writes.
func (s *cachedLikeStorage) Like(ctx context.Context, userID, feedID string) (int64, error) {
	// Already liked — just return current count.
	if s.redis.SIsMember(ctx, likedByKey(feedID), userID).Val() {
		return s.Count(ctx, feedID)
	}

	// First-time like.
	pipe := s.redis.Pipeline()
	incr := pipe.Incr(ctx, likesKey(feedID))
	pipe.SAdd(ctx, likedByKey(feedID), userID)
	pipe.Expire(ctx, likesKey(feedID), likeTTL)
	pipe.Expire(ctx, likedByKey(feedID), likeTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("storage: like: %w", err)
	}

	if err := s.inner.Insert(ctx, userID, feedID); err != nil {
		fmt.Printf("[cachedLikeStorage] db insert failed (non-fatal): %v\n", err)
	}

	return incr.Val(), nil
}

// Unlike removes a like. Checks membership first to avoid unnecessary writes.
func (s *cachedLikeStorage) Unlike(ctx context.Context, userID, feedID string) (int64, error) {
	// Not liked — just return current count.
	if !s.redis.SIsMember(ctx, likedByKey(feedID), userID).Val() {
		return s.Count(ctx, feedID)
	}

	pipe := s.redis.Pipeline()
	decr := pipe.Decr(ctx, likesKey(feedID))
	pipe.SRem(ctx, likedByKey(feedID), userID)
	pipe.Expire(ctx, likesKey(feedID), likeTTL)
	pipe.Expire(ctx, likedByKey(feedID), likeTTL)
	if _, err := pipe.Exec(ctx); err != nil {
		return 0, fmt.Errorf("storage: unlike: %w", err)
	}

	count := decr.Val()
	if count < 0 {
		s.redis.Set(ctx, likesKey(feedID), 0, likeTTL)
		count = 0
	}

	if err := s.inner.Delete(ctx, userID, feedID); err != nil {
		fmt.Printf("[cachedLikeStorage] db delete failed (non-fatal): %v\n", err)
	}

	return count, nil
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

	dbCount, err := s.inner.Count(ctx, feedID)
	if err != nil {
		return 0, err
	}
	s.redis.Set(ctx, likesKey(feedID), dbCount, likeTTL)
	return dbCount, nil
}

// BatchCount returns like counts for multiple feeds in one pipeline round-trip.
// Misses are resolved against the DB concurrently, then backfilled.
func (s *cachedLikeStorage) BatchCount(ctx context.Context, feedIDs []string) (map[string]int64, error) {
	if len(feedIDs) == 0 {
		return map[string]int64{}, nil
	}

	result := make(map[string]int64, len(feedIDs))

	pipe := s.redis.Pipeline()
	cmds := make([]*redis.StringCmd, len(feedIDs))
	for i, id := range feedIDs {
		cmds[i] = pipe.Get(ctx, likesKey(id))
	}
	if _, err := pipe.Exec(ctx); err != nil && err != redis.Nil {
		return nil, fmt.Errorf("storage: batch count: %w", err)
	}

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

	backfill := s.redis.Pipeline()
	for _, mr := range missResults {
		if mr.id == "" {
			continue
		}
		result[mr.id] = mr.count
		backfill.Set(ctx, likesKey(mr.id), mr.count, likeTTL)
	}
	if _, err := backfill.Exec(ctx); err != nil {
		fmt.Printf("[cachedLikeStorage] batch backfill redis: %v\n", err)
	}
	return result, nil
}

// IsLiked checks the Redis set first, falls back to DB.
// When the set has expired, it is fully rebuilt from post_likes.
func (s *cachedLikeStorage) IsLiked(ctx context.Context, userID, feedID string) (bool, error) {
	exists, err := s.redis.SIsMember(ctx, likedByKey(feedID), userID).Result()
	if err == nil {
		return exists, nil
	}
	if err != redis.Nil {
		return false, fmt.Errorf("storage: is liked: %w", err)
	}

	// Set expired — rebuild from DB
	if err := s.rebuildLikedBy(ctx, feedID); err != nil {
		fmt.Printf("[cachedLikeStorage] rebuild liked_by %s: %v\n", feedID, err)
		return s.inner.IsLiked(ctx, userID, feedID)
	}
	// Re-check after rebuild
	return s.redis.SIsMember(ctx, likedByKey(feedID), userID).Result()
}

// rebuildLikedBy repopulates the liked_by set from post_likes in batches.
func (s *cachedLikeStorage) rebuildLikedBy(ctx context.Context, feedID string) error {
	userIDs, err := s.inner.ListLikers(ctx, feedID)
	if err != nil {
		return err
	}
	if len(userIDs) == 0 {
		return nil
	}

	const batchSize = 1000
	key := likedByKey(feedID)

	for start := 0; start < len(userIDs); start += batchSize {
		end := min(start+batchSize, len(userIDs))
		n := end - start

		members := make([]any, n)
		for i, uid := range userIDs[start:end] {
			members[i] = uid
		}

		pipe := s.redis.Pipeline()
		pipe.SAdd(ctx, key, members...)
		// Graduated TTL: non-final batches get a short TTL so partial
		// failures self-heal quickly. The final batch promotes to full TTL.
		if end >= len(userIDs) {
			pipe.Expire(ctx, key, likeTTL)
		} else {
			pipe.Expire(ctx, key, rebuildTTL)
		}
		if _, err := pipe.Exec(ctx); err != nil {
			return fmt.Errorf("storage: rebuild liked_by: %w", err)
		}
	}
	return nil
}
