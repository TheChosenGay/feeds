package main

import (
	"context"
	"embed"
	"log"
	"net"

	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/pkg/storage"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	pb "github.com/TheChosenGay/feeds/proto/gen/feed"
	"google.golang.org/grpc"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	shutdown, err := telemetry.Init(context.Background(), "feed-service")
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer shutdown(context.Background())

	cfg := config.Load("feed")

	db, err := storage.NewPostgresPool(context.Background(), cfg.Postgres.DSN(), 20)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := storage.RunMigrationsFS(cfg.Postgres.MigrateURL(), migrationsFS, "migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	repo := NewFeedRepository(db)
	svc := NewFeedService(repo)

	lis, err := net.Listen("tcp", ":9001")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer(telemetry.GRPCServerOptions(telemetry.StatsHandler())...)
	pb.RegisterFeedServiceServer(s, svc)

	log.Printf("feed service listening on :9001")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
