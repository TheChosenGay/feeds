package srv

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

	"github.com/TheChosenGay/feeds/pkg/auth"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	"github.com/TheChosenGay/feeds/proto/gen/user"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

type UserService struct {
	userSvc user.UserServicevClient
}

func NewUserService() *UserService {
	userAddr := os.Getenv("USER_SERVICE_ADDR")
	if userAddr == "" {
		userAddr = "localhost:9003"
	}
	conn, err := grpc.NewClient(
		userAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithStatsHandler(telemetry.ClientStatsHandler()),
	)
	if err != nil {
		log.Fatalf("failed to create grpc client: %v", err)
	}
	userSvc := user.NewUserServicevClient(conn)

	return &UserService{
		userSvc: userSvc,
	}
}

func (s *UserService) RegisterMux(ctx context.Context, mx *http.ServeMux) {
	mx.HandleFunc("POST /user/register", s.handleRegister)
	mx.HandleFunc("POST /user/login", s.handleLogin)
	mx.HandleFunc("GET /user/{id}", s.handleGetUserInfo)
	mx.HandleFunc("DELETE /user/{id}", s.handleUnregister)
	// follow
	mx.HandleFunc("POST /user/{id}/follow", s.handleFollow)
	mx.HandleFunc("DELETE /user/{id}/follow", s.handleUnfollow)
	mx.HandleFunc("GET /user/{id}/followers", s.handleGetFollowers)
	mx.HandleFunc("GET /user/{id}/following", s.handleGetFollowing)
	mx.HandleFunc("GET /user/{id}/follow", s.handleIsFollowing)
}

func (s *UserService) handleRegister(w http.ResponseWriter, r *http.Request) {
	var req user.RegisterReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	resp, err := s.userSvc.Register(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id": resp.GetId(),
	})
}

func (s *UserService) handleGetUserInfo(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	userInfo, err := s.userSvc.GetInfo(r.Context(), &user.GetInfoReq{Id: id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"id":         userInfo.GetId(),
		"username":   userInfo.GetUsername(),
		"avatar_url": userInfo.GetAvatarUrl(),
	})
}

func (s *UserService) handleLogin(w http.ResponseWriter, r *http.Request) {
	var req user.LoginReq
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	loginResp, err := s.userSvc.Login(r.Context(), &req)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"success": loginResp.GetSuccess(),
		"token":   loginResp.GetToken(),
	}); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
}

func (s *UserService) handleUnregister(w http.ResponseWriter, r *http.Request) {
	userID := auth.UserIDFromContext(r.Context())
	resp, err := s.userSvc.Unregister(r.Context(), &user.UnregisterReq{Id: userID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": resp.GetSuccess(),
	})
}

// --- Follow ---

func (s *UserService) handleFollow(w http.ResponseWriter, r *http.Request) {
	followerID := auth.UserIDFromContext(r.Context())
	followedID := r.PathValue("id")
	resp, err := s.userSvc.Follow(r.Context(), &user.FollowReq{
		FollowerId: followerID,
		FollowedId: followedID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": resp.GetSuccess(),
	})
}

func (s *UserService) handleUnfollow(w http.ResponseWriter, r *http.Request) {
	followerID := auth.UserIDFromContext(r.Context())
	followedID := r.PathValue("id")
	resp, err := s.userSvc.Unfollow(r.Context(), &user.UnfollowReq{
		FollowerId: followerID,
		FollowedId: followedID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": resp.GetSuccess(),
	})
}

func (s *UserService) handleGetFollowers(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	resp, err := s.userSvc.GetFollowers(r.Context(), &user.GetFollowersReq{UserId: userID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_ids": resp.GetUserIds(),
	})
}

func (s *UserService) handleGetFollowing(w http.ResponseWriter, r *http.Request) {
	userID := r.PathValue("id")
	resp, err := s.userSvc.GetFollowing(r.Context(), &user.GetFollowingReq{UserId: userID})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"user_ids": resp.GetUserIds(),
	})
}

func (s *UserService) handleIsFollowing(w http.ResponseWriter, r *http.Request) {
	followerID := auth.UserIDFromContext(r.Context())
	followedID := r.PathValue("id")
	resp, err := s.userSvc.IsFollowing(r.Context(), &user.IsFollowingReq{
		FollowerId: followerID,
		FollowedId: followedID,
	})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"is_following": resp.GetIsFollowing(),
	})
}
