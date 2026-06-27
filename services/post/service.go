package main

import (
	"context"

	pb "github.com/TheChosenGay/feeds/proto/gen/post"
)

type PostService struct {
	pb.UnimplementedPostServiceServer
	repo *PostRepository
}

func NewPostService(repo *PostRepository) *PostService {
	return &PostService{repo: repo}
}

func (s *PostService) CreatePost(ctx context.Context, req *pb.CreatePostReq) (*pb.CreatePostResp, error) {
	postType := req.PostType
	if postType == "" {
		postType = "text"
	}
	p, err := s.repo.Create(ctx, req.AuthorId, req.Content, postType, req.MediaUrls)
	if err != nil {
		return nil, err
	}
	// TODO: emit Kafka "post.created" event
	return &pb.CreatePostResp{Id: p.ID}, nil
}

func (s *PostService) GetPost(ctx context.Context, req *pb.GetPostReq) (*pb.GetPostResp, error) {
	p, err := s.repo.FindByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &pb.GetPostResp{
		Id:        p.ID,
		AuthorId:  p.AuthorID,
		Content:   p.Content,
		PostType:  p.PostType,
		MediaUrls: p.MediaURLs,
		CreatedAt: p.CreatedAt.Unix(),
		UpdatedAt: p.UpdatedAt.Unix(),
	}, nil
}

func (s *PostService) ListPosts(ctx context.Context, req *pb.ListPostsReq) (*pb.ListPostsResp, error) {
	page, pageSize := int(req.Page), int(req.PageSize)
	if page <= 0 {
		page = 1
	}
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	posts, total, err := s.repo.List(ctx, req.AuthorId, page, pageSize)
	if err != nil {
		return nil, err
	}

	resp := &pb.ListPostsResp{Total: int32(total)}
	for _, p := range posts {
		resp.Posts = append(resp.Posts, &pb.GetPostResp{
			Id:        p.ID,
			AuthorId:  p.AuthorID,
			Content:   p.Content,
			PostType:  p.PostType,
			MediaUrls: p.MediaURLs,
			CreatedAt: p.CreatedAt.Unix(),
			UpdatedAt: p.UpdatedAt.Unix(),
		})
	}
	return resp, nil
}

func (s *PostService) DeletePost(ctx context.Context, req *pb.DeletePostReq) (*pb.DeletePostResp, error) {
	if err := s.repo.Delete(ctx, req.Id, req.AuthorId); err != nil {
		return &pb.DeletePostResp{Success: false}, err
	}
	return &pb.DeletePostResp{Success: true}, nil
}
