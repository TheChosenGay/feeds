package srv

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/TheChosenGay/feeds/pkg/auth"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	pb "github.com/TheChosenGay/feeds/proto/gen/interaction"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// InteractionService proxies REST requests to the interaction gRPC service.
type InteractionService struct {
	client pb.InteractionServiceClient
}

func NewInteractionService() *InteractionService {
	addr := os.Getenv("INTERACTION_SERVICE_ADDR")
	if addr == "" {
		addr = "localhost:9005"
	}
	conn, err := grpc.NewClient(addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(telemetry.ClientStatsHandler()),
	)
	if err != nil {
		log.Fatalf("interaction grpc: %v", err)
	}
	return &InteractionService{client: pb.NewInteractionServiceClient(conn)}
}

func (s *InteractionService) RegisterMux(ctx context.Context, mx *http.ServeMux) {
	log.Println("[interaction] registering routes...")
	mx.HandleFunc("POST /feeds/{id}/like", s.handleLike)
	mx.HandleFunc("DELETE /feeds/{id}/like", s.handleUnlike)
	mx.HandleFunc("GET /feeds/{id}/like", s.handleIsLiked)
	mx.HandleFunc("GET /feeds/{id}/likes/count", s.handleCountLikes)
	mx.HandleFunc("POST /feeds/likes/batch", s.handleBatchCountLikes)
	mx.HandleFunc("POST /feeds/{id}/comments", s.handleCreateComment)
	mx.HandleFunc("GET /feeds/{id}/comments", s.handleListComments)
	mx.HandleFunc("DELETE /comments/{id}", s.handleDeleteComment)
	mx.HandleFunc("POST /feeds/{id}/bookmark", s.handleBookmark)
	mx.HandleFunc("DELETE /feeds/{id}/bookmark", s.handleUnbookmark)
	mx.HandleFunc("GET /feeds/{id}/bookmark", s.handleIsBookmarked)
	log.Println("[interaction] routes registered")
}

// --- Like ---

func (s *InteractionService) handleLike(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.Like(r.Context(), &pb.LikeReq{UserId: userID, PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleUnlike(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.Unlike(r.Context(), &pb.UnlikeReq{UserId: userID, PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleIsLiked(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.IsLiked(r.Context(), &pb.IsLikedReq{UserId: userID, PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleCountLikes(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	resp, err := s.client.CountLikes(r.Context(), &pb.CountLikesReq{PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleBatchCountLikes(w http.ResponseWriter, r *http.Request) {
	var body struct {
		PostIDs []string `json:"post_ids"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := s.client.BatchCountLikes(r.Context(), &pb.BatchCountLikesReq{PostIds: body.PostIDs})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

// --- Comment ---

func (s *InteractionService) handleCreateComment(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	var body struct {
		Content string `json:"content"`
	}
	json.NewDecoder(r.Body).Decode(&body)
	resp, err := s.client.CreateComment(r.Context(), &pb.CreateCommentReq{
		UserId:  userID,
		PostId:  feedID,
		Content: body.Content,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleListComments(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	page := parseInt(r.URL.Query().Get("page"), 1)
	pageSize := parseInt(r.URL.Query().Get("page_size"), 20)

	resp, err := s.client.ListComments(r.Context(), &pb.ListCommentsReq{
		PostId:   feedID,
		Page:     int32(page),
		PageSize: int32(pageSize),
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleDeleteComment(w http.ResponseWriter, r *http.Request) {
	commentID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.DeleteComment(r.Context(), &pb.DeleteCommentReq{Id: commentID, UserId: userID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

// --- Bookmark ---

func (s *InteractionService) handleBookmark(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.Bookmark(r.Context(), &pb.BookmarkReq{UserId: userID, PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleUnbookmark(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.Unbookmark(r.Context(), &pb.UnbookmarkReq{UserId: userID, PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}

func (s *InteractionService) handleIsBookmarked(w http.ResponseWriter, r *http.Request) {
	feedID := r.PathValue("id")
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.client.IsBookmarked(r.Context(), &pb.IsBookmarkedReq{UserId: userID, PostId: feedID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	writeJSON(w, resp)
}
