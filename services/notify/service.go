package main

import (
	"context"
	"encoding/json"
	"log"

	cometpb "github.com/TheChosenGay/feeds/proto/gen/comet"
	pb "github.com/TheChosenGay/feeds/proto/gen/notify"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

type notifyService struct {
	pb.UnimplementedNotifyServiceServer
	store    *notifyStore
	cometCli cometpb.CometServiceClient
}

func NewNotifyService(store *notifyStore, cometCli cometpb.CometServiceClient) *notifyService {
	return &notifyService{store: store, cometCli: cometCli}
}

// Push 推送通知：存 DB + comet 实时投递。
func (s *notifyService) Push(ctx context.Context, req *pb.PushReq) (*pb.PushResp, error) {
	if req.UserId == "" || req.Type == "" || req.Title == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id, type, title are required")
	}

	// 1. 写入数据库
	id, err := s.store.Insert(ctx, &Notification{
		UserID:  req.UserId,
		Type:    req.Type,
		Title:   req.Title,
		Body:    req.Body,
		Payload: req.Payload,
	})
	if err != nil {
		log.Printf("[notify] insert error: %v", err)
		return nil, status.Errorf(codes.Internal, "insert: %v", err)
	}

	// 2. 通过 comet 实时推送
	wsPayload, _ := json.Marshal(map[string]interface{}{
		"id":         id,
		"type":       req.Type,
		"title":      req.Title,
		"body":       req.Body,
		"payload":    req.Payload,
		"created_at": nil, // 客户端自行处理时间
	})

	cometResp, err := s.cometCli.PushUser(ctx, &cometpb.PushUserReq{
		UserId:  req.UserId,
		Payload: wsPayload,
	})
	if err != nil {
		log.Printf("[notify] comet push error (stored ok): %v", err)
		return &pb.PushResp{Id: id, DeliveredWs: false, DeviceCount: 0}, nil
	}

	return &pb.PushResp{
		Id:           id,
		DeliveredWs:  cometResp.Delivered > 0,
		DeviceCount:  cometResp.Delivered,
	}, nil
}

// ListNotifications 查询通知历史。
func (s *notifyService) ListNotifications(ctx context.Context, req *pb.ListNotificationsReq) (*pb.ListNotificationsResp, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}

	pageSize := req.PageSize
	if pageSize <= 0 || pageSize > 50 {
		pageSize = 20
	}

	items, nextCursor, err := s.store.ListByUser(ctx, req.UserId, req.Cursor, pageSize)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "list: %v", err)
	}

	resp := &pb.ListNotificationsResp{NextCursor: nextCursor}
	for _, n := range items {
		resp.Items = append(resp.Items, &pb.NotificationItem{
			Id:        n.ID,
			Type:      n.Type,
			Title:     n.Title,
			Body:      n.Body,
			Payload:   n.Payload,
			IsRead:    n.ReadAt != nil,
			CreatedAt: n.CreatedAt.Unix(),
		})
	}
	return resp, nil
}

// MarkRead 标记已读。
func (s *notifyService) MarkRead(ctx context.Context, req *pb.MarkReadReq) (*pb.MarkReadResp, error) {
	if req.UserId == "" || req.NotificationId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id and notification_id are required")
	}
	if err := s.store.MarkRead(ctx, req.UserId, req.NotificationId); err != nil {
		return nil, status.Errorf(codes.Internal, "mark_read: %v", err)
	}
	return &pb.MarkReadResp{Ok: true}, nil
}

// GetUnreadCount 获取未读数。
func (s *notifyService) GetUnreadCount(ctx context.Context, req *pb.GetUnreadCountReq) (*pb.GetUnreadCountResp, error) {
	if req.UserId == "" {
		return nil, status.Error(codes.InvalidArgument, "user_id is required")
	}
	count, err := s.store.UnreadCount(ctx, req.UserId)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "unread_count: %v", err)
	}
	return &pb.GetUnreadCountResp{Count: count}, nil
}
