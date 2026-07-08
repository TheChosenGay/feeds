package main

import (
	"context"
	"embed"
	"log"
	"net"

	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/pkg/storage"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	cometpb "github.com/TheChosenGay/feeds/proto/gen/comet"
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

	cometAddr := config.GetEnv("NOTIFY_COMET_ADDR", "localhost:9006")
	cometConn, err := grpc.NewClient(cometAddr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		log.Fatalf("comet grpc dial: %v", err)
	}
	defer cometConn.Close()

	cometCli := cometpb.NewLiveServiceClient(cometConn)
	svc := NewNotifyService(&notifyStore{db: db}, cometCli)

	lis, err := net.Listen("tcp", ":9007")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer(telemetry.GRPCServerOptions(telemetry.StatsHandler())...)
	pb.RegisterNotifyServiceServer(s, svc)

	log.Printf("notify service listening on :9007 (comet=%s)", cometAddr)
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
