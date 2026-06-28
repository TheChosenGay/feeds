package main

import (
	"context"
	"log"
	"net/http"

	"github.com/TheChosenGay/feeds/pkg/auth"
	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	"github.com/TheChosenGay/feeds/services/gateway/srv"
)

func main() {
	shutdown, err := telemetry.Init(context.Background(), "gateway")
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer shutdown(context.Background())

	cfg := config.Load("")
	_ = cfg

	mux := http.NewServeMux()
	upload := srv.NewUploadHandler(cfg.COS)
	upload.RegisterMux(context.Background(), mux)

	svcManager := NewServiceManager()
	svcManager.RegisterService(srv.NewUserService())
	svcManager.RegisterService(srv.NewFeedService())
	svcManager.RegisterService(srv.NewInteractionService())
	mux = svcManager.HandleMux(mux)

	publicPaths := []string{"/user/register", "/user/login"}
	var handler http.Handler = auth.Middleware(mux.ServeHTTP, publicPaths)
	// otel http middleware (outer — traces everything including auth)
	handler = telemetry.HTTPMiddleware(handler, "gateway")

	addr := ":8080"
	log.Printf("gateway listening on %s", addr)
	if err := http.ListenAndServe(addr, handler); err != nil {
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
