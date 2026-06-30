package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/TheChosenGay/feeds/pkg/events"
	pb "github.com/TheChosenGay/feeds/proto/gen/feed"
)

type FeedService struct {
	pb.UnimplementedFeedServiceServer
	repo       *FeedRepository
	dispatcher events.Dispatcher
}

func NewFeedService(repo *FeedRepository, disp events.Dispatcher) *FeedService {
	if disp == nil {
		disp = events.NewNoopDispatcher()
	}
	return &FeedService{repo: repo, dispatcher: disp}
}

func (s *FeedService) PostFeed(ctx context.Context, req *pb.PostFeedReq) (*pb.PostFeedResp, error) {
	blocks := blocksFromProto(req.Blocks)
	f, err := s.repo.Create(ctx, req.AuthorId, blocks)
	if err != nil {
		return nil, err
	}

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
	f, err := s.repo.FindByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.GetFeedResp{
		Id:        f.ID,
		AuthorId:  f.AuthorID,
		Blocks:    blocksToProto(f.Blocks),
		CreatedAt: f.CreatedAt.Unix(),
		UpdatedAt: f.UpdatedAt.Unix(),
	}, nil
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
