package srv

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/TheChosenGay/feeds/pkg/auth"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	pb "github.com/TheChosenGay/feeds/proto/gen/feed"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type FeedService struct {
	feedSvc pb.FeedServiceClient
}

func NewFeedService() *FeedService {
	addr := os.Getenv("FEED_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9001"
	}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(telemetry.ClientStatsHandler()),
	)
	if err != nil {
		log.Fatalf("feed grpc: %v", err)
	}
	return &FeedService{feedSvc: pb.NewFeedServiceClient(conn)}
}

func (s *FeedService) RegisterMux(ctx context.Context, mx *http.ServeMux) {
	log.Println("[feed] registering routes...")
	mx.HandleFunc("POST /feeds", s.handlePostFeed)
	mx.HandleFunc("GET /feeds/{id}", s.handleGetFeed)
	mx.HandleFunc("GET /feeds", s.handleListFeeds)
	mx.HandleFunc("DELETE /feeds/{id}", s.handleDeleteFeed)
	mx.HandleFunc("GET /feeds/timeline", s.handleGetTimeline)
	log.Println("[feed] routes registered: POST /feeds, GET /feeds/timeline, GET /feeds/{id}, GET /feeds, DELETE /feeds/{id}")
}

func (s *FeedService) handlePostFeed(w http.ResponseWriter, r *http.Request) {
	var body struct {
		Blocks []*pb.Block `json:"blocks"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	req := &pb.PostFeedReq{
		AuthorId: auth.UserIDFromContext(r.Context()),
		Blocks:   body.Blocks,
	}
	resp, err := s.feedSvc.PostFeed(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *FeedService) handleGetFeed(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.feedSvc.GetFeed(r.Context(), &pb.GetFeedReq{Id: id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *FeedService) handleListFeeds(w http.ResponseWriter, r *http.Request) {
	page := parseInt(r.URL.Query().Get("page"), 1)
	pageSize := parseInt(r.URL.Query().Get("page_size"), 20)
	authorID := r.URL.Query().Get("author_id")

	resp, err := s.feedSvc.ListFeeds(r.Context(), &pb.ListFeedsReq{
		AuthorId: authorID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *FeedService) handleDeleteFeed(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	req := &pb.DeleteFeedReq{Id: id, AuthorId: auth.UserIDFromContext(r.Context())}
	resp, err := s.feedSvc.DeleteFeed(r.Context(), req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *FeedService) handleGetTimeline(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	cursor := parseInt(r.URL.Query().Get("cursor"), 0)
	pageSize := parseInt(r.URL.Query().Get("page_size"), 20)

	resp, err := s.feedSvc.GetTimeline(r.Context(), &pb.GetTimelineReq{
		UserId:   userID,
		Cursor:   int64(cursor),
		PageSize: int32(pageSize),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func parseInt(s string, def int) int {
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	if n == 0 {
		return def
	}
	return n
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}
