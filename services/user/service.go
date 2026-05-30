package main

import (
	"context"

	"github.com/TheChosenGay/feeds/proto/gen/user"
)

type UserService struct {
	user.UnimplementedUserServicevServer
}

func NewUserService() user.UserServicevServer {
	return &UserService{}
}

// MARK: - UserServiceServer

func (s *UserService) Register(ctx context.Context, req *user.RegisterReq) (*user.RegisterResp, error) {
	return &user.RegisterResp{
		Id: "123",
	}, nil
}

func (s *UserService) GetInfo(ctx context.Context, req *user.GetInfoReq) (*user.GetInfoResp, error) {
	return &user.GetInfoResp{
		Id:        "123",
		Username:  "test",
		AvatarUrl: "https://example.com/avatar.png",
	}, nil
}

func (s *UserService) Unregister(ctx context.Context, req *user.UnregisterReq) (*user.UnregisterResp, error) {
	return &user.UnregisterResp{
		Success: true,
	}, nil
}

func (s *UserService) Login(ctx context.Context, req *user.LoginReq) (*user.LoginResp, error) {
	return &user.LoginResp{
		Success: true,
		Token:   "123",
	}, nil
}
