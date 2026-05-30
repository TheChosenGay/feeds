package main

import (
	"context"
	"log"
	"net/http"

	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/services/gateway/srv"
)

func main() {
	cfg := config.Load()
	_ = cfg

	mux := http.NewServeMux()
	svc_manager := NewServiceManager()
	svc_manager.RegisterService(srv.NewUserService())
	mux = svc_manager.HandleMux(mux)

	addr := ":8080"
	log.Printf("gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatal(err)
	}
}

type ServiceManager struct {
	services []srv.Service
}

func NewServiceManager() *ServiceManager {
	return &ServiceManager{}
}

func (s *ServiceManager) RegisterService(svc srv.Service) {
	s.services = append(s.services, svc)
}

func (s *ServiceManager) HandleMux(mx *http.ServeMux) *http.ServeMux {
	for _, svc := range s.services {
		svc.RegisterMux(context.Background(), mx)
	}
	return mx
}
