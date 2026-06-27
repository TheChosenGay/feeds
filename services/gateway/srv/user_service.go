package srv

import (
	"context"
	"encoding/json"
	"log"
	"net/http"
	"os"

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
		grpc.WithTransportCredentials(insecure.NewCredentials()))
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
	mx.HandleFunc("GET /user/info/{id}", s.handleGetUserInfo)
	mx.HandleFunc("DELETE /user/{id}", s.handleUnregister)
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
	id := r.PathValue("id")
	resp, err := s.userSvc.Unregister(r.Context(), &user.UnregisterReq{Id: id})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"success": resp.GetSuccess(),
	})
}
