package main

import (
	"context"
	"encoding/json"
	"log"

	"github.com/TheChosenGay/feeds/pkg/events"
	pb "github.com/TheChosenGay/feeds/proto/gen/interaction"
)

// InteractionService implements the gRPC InteractionServiceServer.
type InteractionService struct {
	pb.UnimplementedInteractionServiceServer
	likes        *cachedLikeStorage
	commentRepo  *CommentRepo
	bookmarkRepo *BookmarkRepo
	dispatcher   events.Dispatcher
}

func NewInteractionService(likes *cachedLikeStorage, commentRepo *CommentRepo, bookmarkRepo *BookmarkRepo, disp events.Dispatcher) *InteractionService {
	if disp == nil {
		disp = events.NewNoopDispatcher()
	}
	return &InteractionService{
		likes:        likes,
		commentRepo:  commentRepo,
		bookmarkRepo: bookmarkRepo,
		dispatcher:   disp,
	}
}

func likeEventBody(userID, feedID string) []byte {
	b, _ := json.Marshal(map[string]string{"user_id": userID, "feed_id": feedID})
	return b
}

// --- Like ---

func (s *InteractionService) Like(ctx context.Context, req *pb.LikeReq) (*pb.LikeResp, error) {
	count, err := s.likes.Like(ctx, req.UserId, req.PostId)
	if err != nil {
		return nil, err
	}
	log.Printf("[interaction] like: user=%s post=%s count=%d", req.UserId, req.PostId, count)

	s.dispatcher.Dispatch(ctx, events.Event{
		Topic: "feed.liked",
		Key:   req.PostId,
		Body:  likeEventBody(req.UserId, req.PostId),
	})

	return &pb.LikeResp{Success: true, LikeCount: count}, nil
}

func (s *InteractionService) Unlike(ctx context.Context, req *pb.UnlikeReq) (*pb.UnlikeResp, error) {
	count, err := s.likes.Unlike(ctx, req.UserId, req.PostId)
	if err != nil {
		return nil, err
	}
	log.Printf("[interaction] unlike: user=%s post=%s count=%d", req.UserId, req.PostId, count)

	s.dispatcher.Dispatch(ctx, events.Event{
		Topic: "feed.unliked",
		Key:   req.PostId,
		Body:  likeEventBody(req.UserId, req.PostId),
	})

	return &pb.UnlikeResp{Success: true, LikeCount: count}, nil
}

func (s *InteractionService) IsLiked(ctx context.Context, req *pb.IsLikedReq) (*pb.IsLikedResp, error) {
	liked, err := s.likes.IsLiked(ctx, req.UserId, req.PostId)
	if err != nil {
		return nil, err
	}
	return &pb.IsLikedResp{Liked: liked}, nil
}

func (s *InteractionService) CountLikes(ctx context.Context, req *pb.CountLikesReq) (*pb.CountLikesResp, error) {
	count, err := s.likes.Count(ctx, req.PostId)
	if err != nil {
		return nil, err
	}
	return &pb.CountLikesResp{Count: count}, nil
}

func (s *InteractionService) BatchCountLikes(ctx context.Context, req *pb.BatchCountLikesReq) (*pb.BatchCountLikesResp, error) {
	counts, err := s.likes.BatchCount(ctx, req.PostIds)
	if err != nil {
		return nil, err
	}
	return &pb.BatchCountLikesResp{Counts: counts}, nil
}

// --- Comment ---

func (s *InteractionService) CreateComment(ctx context.Context, req *pb.CreateCommentReq) (*pb.CreateCommentResp, error) {
	c, err := s.commentRepo.Create(ctx, req.UserId, req.PostId, req.Content)
	if err != nil {
		return nil, err
	}
	log.Printf("[interaction] comment created: id=%s user=%s post=%s", c.ID, req.UserId, req.PostId)
	return &pb.CreateCommentResp{
		Comment: &pb.Comment{
			Id:        c.ID,
			UserId:    c.UserID,
			PostId:    c.PostID,
			Content:   c.Content,
			CreatedAt: c.CreatedAt.Unix(),
		},
	}, nil
}

func (s *InteractionService) ListComments(ctx context.Context, req *pb.ListCommentsReq) (*pb.ListCommentsResp, error) {
	page, pageSize := int(req.Page), int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	comments, total, err := s.commentRepo.ListByPost(ctx, req.PostId, page, pageSize)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListCommentsResp{Total: int32(total)}
	for _, c := range comments {
		resp.Comments = append(resp.Comments, &pb.Comment{
			Id:        c.ID,
			UserId:    c.UserID,
			PostId:    c.PostID,
			Content:   c.Content,
			CreatedAt: c.CreatedAt.Unix(),
		})
	}
	return resp, nil
}

func (s *InteractionService) DeleteComment(ctx context.Context, req *pb.DeleteCommentReq) (*pb.DeleteCommentResp, error) {
	if err := s.commentRepo.Delete(ctx, req.Id, req.UserId); err != nil {
		return &pb.DeleteCommentResp{Success: false}, err
	}
	return &pb.DeleteCommentResp{Success: true}, nil
}

// --- Bookmark ---

func (s *InteractionService) Bookmark(ctx context.Context, req *pb.BookmarkReq) (*pb.BookmarkResp, error) {
	if err := s.bookmarkRepo.Bookmark(ctx, req.UserId, req.PostId); err != nil {
		return &pb.BookmarkResp{Success: false}, err
	}
	log.Printf("[interaction] bookmark: user=%s post=%s", req.UserId, req.PostId)
	return &pb.BookmarkResp{Success: true}, nil
}

func (s *InteractionService) Unbookmark(ctx context.Context, req *pb.UnbookmarkReq) (*pb.UnbookmarkResp, error) {
	if err := s.bookmarkRepo.Unbookmark(ctx, req.UserId, req.PostId); err != nil {
		return &pb.UnbookmarkResp{Success: false}, err
	}
	return &pb.UnbookmarkResp{Success: true}, nil
}

func (s *InteractionService) IsBookmarked(ctx context.Context, req *pb.IsBookmarkedReq) (*pb.IsBookmarkedResp, error) {
	bookmarked, err := s.bookmarkRepo.IsBookmarked(ctx, req.UserId, req.PostId)
	if err != nil {
		return nil, err
	}
	return &pb.IsBookmarkedResp{Bookmarked: bookmarked}, nil
}
