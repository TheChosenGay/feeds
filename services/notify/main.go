package main

import (
	"context"
	"embed"
	"log"
	"net"

	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/pkg/storage"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	livepb "github.com/TheChosenGay/feeds/proto/gen/comet"
	pb "github.com/TheChosenGay/feeds/proto/gen/notify"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	shutdown, err := telemetry.Init(context.Background(), "notify")
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer shutdown(context.Background())

	cfg := config.Load("notify")

	db, err := storage.NewPostgresPool(context.Background(), cfg.Postgres.DSN(), 20)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := storage.RunMigrationsFS(cfg.Postgres.MigrateURL(), migrationsFS, "migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	liveAddr := config.GetEnv("NOTIFY_LIVE_ADDR", "localhost:9006")
	liveConn, err := grpc.NewClient(liveAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("live gRPC dial: %v", err)
	}
	defer liveConn.Close()

	liveCli := livepb.NewLiveServiceClient(liveConn)
	svc := NewNotifyService(&notifyStore{db: db}, liveCli)

	lis, err := net.Listen("tcp", ":9007")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer(telemetry.GRPCServerOptions(telemetry.StatsHandler())...)
	pb.RegisterNotifyServiceServer(s, svc)

	log.Printf("notify service listening on :9007 (live=%s)", liveAddr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
