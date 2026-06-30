package main

import (
	"context"
	"database/sql"
	"errors"
	"time"

	"github.com/TheChosenGay/feeds/proto/gen/user"
	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/crypto/bcrypt"
)

var jwtSecret = []byte("feeds-dev-secret") // TODO: load from config/env

type UserService struct {
	user.UnimplementedUserServicevServer
	repo       *UserRepository
	followRepo *FollowRepo
}

func NewUserService(repo *UserRepository, followRepo *FollowRepo) *UserService {
	return &UserService{repo: repo, followRepo: followRepo}
}

func (s *UserService) Register(ctx context.Context, req *user.RegisterReq) (*user.RegisterResp, error) {
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		return nil, err
	}

	u, err := s.repo.Create(ctx, req.Username, string(hash))
	if err != nil {
		return nil, err
	}
	return &user.RegisterResp{Id: u.ID}, nil
}

func (s *UserService) Login(ctx context.Context, req *user.LoginReq) (*user.LoginResp, error) {
	u, err := s.repo.FindByUsername(ctx, req.Username)
	if errors.Is(err, sql.ErrNoRows) {
		return &user.LoginResp{Success: false}, nil
	}
	if err != nil {
		return nil, err
	}
	if bcrypt.CompareHashAndPassword([]byte(u.Password), []byte(req.Password)) != nil {
		return &user.LoginResp{Success: false}, nil
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"user_id":  u.ID,
		"username": u.Username,
		"exp":      time.Now().Add(72 * time.Hour).Unix(),
	})
	signed, err := token.SignedString(jwtSecret)
	if err != nil {
		return nil, err
	}
	return &user.LoginResp{Success: true, Token: signed}, nil
}

func (s *UserService) GetInfo(ctx context.Context, req *user.GetInfoReq) (*user.GetInfoResp, error) {
	u, err := s.repo.FindByID(ctx, req.Id)
	if err != nil {
		return nil, err
	}
	return &user.GetInfoResp{
		Id:        u.ID,
		Username:  u.Username,
		AvatarUrl: u.AvatarURL,
	}, nil
}

func (s *UserService) Unregister(ctx context.Context, req *user.UnregisterReq) (*user.UnregisterResp, error) {
	if err := s.repo.Delete(ctx, req.Id); err != nil {
		return &user.UnregisterResp{Success: false}, err
	}
	return &user.UnregisterResp{Success: true}, nil
}

// --- Follow ---

func (s *UserService) Follow(ctx context.Context, req *user.FollowReq) (*user.FollowResp, error) {
	if err := s.followRepo.Follow(ctx, req.FollowerId, req.FollowedId); err != nil {
		return &user.FollowResp{Success: false}, err
	}
	return &user.FollowResp{Success: true}, nil
}

func (s *UserService) Unfollow(ctx context.Context, req *user.UnfollowReq) (*user.UnfollowResp, error) {
	if err := s.followRepo.Unfollow(ctx, req.FollowerId, req.FollowedId); err != nil {
		return &user.UnfollowResp{Success: false}, err
	}
	return &user.UnfollowResp{Success: true}, nil
}

func (s *UserService) GetFollowers(ctx context.Context, req *user.GetFollowersReq) (*user.GetFollowersResp, error) {
	ids, err := s.followRepo.GetFollowers(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &user.GetFollowersResp{UserIds: ids}, nil
}

func (s *UserService) GetFollowing(ctx context.Context, req *user.GetFollowingReq) (*user.GetFollowingResp, error) {
	ids, err := s.followRepo.GetFollowing(ctx, req.UserId)
	if err != nil {
		return nil, err
	}
	return &user.GetFollowingResp{UserIds: ids}, nil
}

func (s *UserService) IsFollowing(ctx context.Context, req *user.IsFollowingReq) (*user.IsFollowingResp, error) {
	ok, err := s.followRepo.IsFollowing(ctx, req.FollowerId, req.FollowedId)
	if err != nil {
		return nil, err
	}
	return &user.IsFollowingResp{IsFollowing: ok}, nil
}
