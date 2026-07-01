package main

import (
	"context"
	"encoding/json"
	"log"
	"time"

	"github.com/TheChosenGay/feeds/pkg/events"
	pb "github.com/TheChosenGay/feeds/proto/gen/feed"
	"github.com/redis/go-redis/v9"
)

// Slot ratio: 4 inbox posts → 1 hot post.
const inboxRatio = 4

var hotPostsKey = "hot_posts"
var postCacheTTL = 10 * time.Minute

func postCacheKey(id string) string { return "post:" + id }

type FeedService struct {
	pb.UnimplementedFeedServiceServer
	repo       *FeedRepository
	dispatcher events.Dispatcher
	rdb        *redis.Client
}

func NewFeedService(repo *FeedRepository, disp events.Dispatcher, rdb *redis.Client) *FeedService {
	if disp == nil {
		disp = events.NewNoopDispatcher()
	}
	return &FeedService{repo: repo, dispatcher: disp, rdb: rdb}
}

func (s *FeedService) PostFeed(ctx context.Context, req *pb.PostFeedReq) (*pb.PostFeedResp, error) {
	blocks := blocksFromProto(req.Blocks)
	f, err := s.repo.Create(ctx, req.AuthorId, blocks)
	if err != nil {
		return nil, err
	}

	// Cache the new post immediately so it's available without a DB hit.
	resp := &pb.GetFeedResp{
		Id:        f.ID,
		AuthorId:  f.AuthorID,
		Blocks:    blocksToProto(f.Blocks),
		CreatedAt: f.CreatedAt.Unix(),
		UpdatedAt: f.UpdatedAt.Unix(),
	}
	data, _ := json.Marshal(resp)
	s.rdb.Set(ctx, postCacheKey(f.ID), data, postCacheTTL)

	// Fire-and-forget: notify fanout workers so they can push to follower inboxes.
	body, _ := json.Marshal(map[string]interface{}{
		"post_id":        f.ID,
		"author_id":      req.AuthorId,
		"created_at":     f.CreatedAt.Unix(),
		"follower_count": 0, // TODO: query user service for actual count
	})
	s.dispatcher.Dispatch(ctx, events.Event{
		Topic: "post.created",
		Key:   f.ID,
		Body:  body,
	})
	log.Printf("[feed] post.created dispatched: id=%s author=%s", f.ID, req.AuthorId)

	return &pb.PostFeedResp{Id: f.ID}, nil
}

func (s *FeedService) GetFeed(ctx context.Context, req *pb.GetFeedReq) (*pb.GetFeedResp, error) {
	return s.getPostContent(ctx, req.Id)
}

// getPostContent is the shared post fetch: Redis cache → DB fallback.
// Timeline and GetFeed both call this, so hot posts cached by ranking worker
// are served directly from Redis without touching PostgreSQL.
func (s *FeedService) getPostContent(ctx context.Context, id string) (*pb.GetFeedResp, error) {
	// 1. Redis cache
	cached, err := s.rdb.Get(ctx, postCacheKey(id)).Result()
	if err == nil {
		var resp pb.GetFeedResp
		if err := json.Unmarshal([]byte(cached), &resp); err == nil {
			return &resp, nil
		}
		// corrupted cache → fall through to DB
	}

	// 2. DB
	f, err := s.repo.FindByID(ctx, id)
	if err != nil {
		return nil, err
	}

	resp := &pb.GetFeedResp{
		Id:        f.ID,
		AuthorId:  f.AuthorID,
		Blocks:    blocksToProto(f.Blocks),
		CreatedAt: f.CreatedAt.Unix(),
		UpdatedAt: f.UpdatedAt.Unix(),
	}

	// 3. Cache
	data, _ := json.Marshal(resp)
	s.rdb.Set(ctx, postCacheKey(id), data, postCacheTTL)

	return resp, nil
}

func (s *FeedService) ListFeeds(ctx context.Context, req *pb.ListFeedsReq) (*pb.ListFeedsResp, error) {
	page, pageSize := int(req.Page), int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	feeds, total, err := s.repo.List(ctx, req.AuthorId, page, pageSize)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListFeedsResp{Total: int32(total)}
	for _, f := range feeds {
		resp.Feeds = append(resp.Feeds, &pb.GetFeedResp{
			Id:        f.ID,
			AuthorId:  f.AuthorID,
			Blocks:    blocksToProto(f.Blocks),
			CreatedAt: f.CreatedAt.Unix(),
			UpdatedAt: f.UpdatedAt.Unix(),
		})
	}
	return resp, nil
}

func (s *FeedService) DeleteFeed(ctx context.Context, req *pb.DeleteFeedReq) (*pb.DeleteFeedResp, error) {
	if err := s.repo.Delete(ctx, req.Id, req.AuthorId); err != nil {
		return &pb.DeleteFeedResp{Success: false}, err
	}
	return &pb.DeleteFeedResp{Success: true}, nil
}

// GetTimeline builds a personalized feed from inbox + hot_posts with slot-based
// interleaving (4 inbox : 1 hot). Cursor doubles as page number (0 = first page).
func (s *FeedService) GetTimeline(ctx context.Context, req *pb.GetTimelineReq) (*pb.GetTimelineResp, error) {
	page := int(req.Cursor)
	if page <= 0 {
		page = 1
	}
	pageSize := int(req.PageSize)
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	// How many from each source (per page, accounting for interleave ratio).
	hotPerPage := pageSize / (inboxRatio + 1)
	if hotPerPage < 1 {
		hotPerPage = 1
	}
	inboxPerPage := pageSize - hotPerPage

	inboxKey := "inbox:" + req.UserId

	// Fetch from both ZSETs (score desc = newest first).
	inboxStart := int64((page - 1) * inboxPerPage)
	inboxStop := inboxStart + int64(inboxPerPage) + int64(hotPerPage) // extra for dedup

	hotStart := int64((page - 1) * hotPerPage)
	hotStop := hotStart + int64(hotPerPage)

	inboxItems, err := s.rdb.ZRevRangeWithScores(ctx, inboxKey, inboxStart, inboxStop-1).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[feed] timeline inbox error: %v", err)
	}
	hotItems, err := s.rdb.ZRevRangeWithScores(ctx, hotPostsKey, hotStart, hotStop-1).Result()
	if err != nil && err != redis.Nil {
		log.Printf("[feed] timeline hot error: %v", err)
	}

	// Interleave: put 1 hot every inboxRatio inbox posts.
	merged := interleave(inboxItems, hotItems, inboxRatio)
	if len(merged) > pageSize {
		merged = merged[:pageSize]
	}

	// Collect unique post IDs (dedup).
	ids := make([]string, 0, len(merged))
	seen := make(map[string]bool, len(merged))
	for _, m := range merged {
		if seen[m.postID] {
			continue
		}
		seen[m.postID] = true
		ids = append(ids, m.postID)
	}

	// Fetch content via getPostContent (Redis cache → DB fallback).
	// Hot posts pre-cached by ranking worker are served from Redis.
	resp := &pb.GetTimelineResp{
		NextCursor: int64(page + 1),
		HasMore:    len(merged) >= pageSize,
	}
	for _, id := range ids {
		feed, err := s.getPostContent(ctx, id)
		if err != nil {
			log.Printf("[feed] timeline getPostContent(%s): %v", id, err)
			continue
		}
		resp.Feeds = append(resp.Feeds, feed)
	}
	return resp, nil
}

// interleaveSlot holds a single post reference from a ZSET.
type interleaveSlot struct {
	postID string
	score  float64
	source string // "inbox" or "hot"
}

// interleave merges inbox and hot items with a slot pattern.
// Every ratio inbox items are followed by 1 hot item.
func interleave(inbox, hot []redis.Z, ratio int) []interleaveSlot {
	result := make([]interleaveSlot, 0, len(inbox)+len(hot))
	hi := 0

	for i, z := range inbox {
		result = append(result, interleaveSlot{postID: z.Member.(string), score: z.Score, source: "inbox"})

		// After every 'ratio' inbox items, insert a hot item.
		if (i+1)%ratio == 0 && hi < len(hot) {
			result = append(result, interleaveSlot{postID: hot[hi].Member.(string), score: hot[hi].Score, source: "hot"})
			hi++
		}
	}
	// Append remaining hot items at the end.
	for hi < len(hot) {
		result = append(result, interleaveSlot{postID: hot[hi].Member.(string), score: hot[hi].Score, source: "hot"})
		hi++
	}
	return result
}

func blocksFromProto(pbs []*pb.Block) Blocks {
	blocks := make(Blocks, len(pbs))
	for i, b := range pbs {
		blocks[i] = Block{
			Type:     b.Type,
			Content:  b.Content,
			URL:      b.Url,
			CoverURL: b.CoverUrl,
			Width:    b.Width,
			Height:   b.Height,
			Duration: b.Duration,
			Size:     b.Size,
		}
	}
	return blocks
}

func blocksToProto(blocks Blocks) []*pb.Block {
	pbs := make([]*pb.Block, len(blocks))
	for i, b := range blocks {
		pbs[i] = &pb.Block{
			Type:     b.Type,
			Content:  b.Content,
			Url:      b.URL,
			CoverUrl: b.CoverURL,
			Width:    b.Width,
			Height:   b.Height,
			Duration: b.Duration,
			Size:     b.Size,
		}
	}
	return pbs
}
