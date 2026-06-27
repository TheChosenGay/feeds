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
	repo *UserRepository
}

func NewUserService(repo *UserRepository) *UserService {
	return &UserService{repo: repo}
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
