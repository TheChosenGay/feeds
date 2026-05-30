package main

import (
	"log"
	"net"

	"github.com/TheChosenGay/feeds/proto/gen/user"
	"google.golang.org/grpc"
)

func main() {
	lis, err := net.Listen("tcp", ":9003")
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	s := grpc.NewServer()

	user.RegisterUserServicevServer(s, NewUserService())

	if err := s.Serve(lis); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
}
