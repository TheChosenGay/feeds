package main

import (
	"context"
	"embed"
	"log"
	"net"

	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/pkg/events"
	"github.com/TheChosenGay/feeds/pkg/storage"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	pb "github.com/TheChosenGay/feeds/proto/gen/interaction"
	"github.com/redis/go-redis/extra/redisotel/v9"
	"google.golang.org/grpc"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

func main() {
	shutdown, err := telemetry.Init(context.Background(), "interaction-service")
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer shutdown(context.Background())

	cfg := config.Load("interaction")

	db, err := storage.NewPostgresPool(context.Background(), cfg.Postgres.DSN(), 20)
	if err != nil {
		log.Fatalf("db: %v", err)
	}
	defer db.Close()

	if err := storage.RunMigrationsFS(cfg.Postgres.MigrateURL(), migrationsFS, "migrations"); err != nil {
		log.Fatalf("migrations: %v", err)
	}

	rdb, err := storage.NewRedisClient(cfg.Redis.Addr, cfg.Redis.Password, cfg.Redis.DB)
	if err != nil {
		log.Fatalf("redis: %v", err)
	}
	defer rdb.Close()

	// OpenTelemetry instrumentation for Redis (traces + metrics)
	if err := redisotel.InstrumentTracing(rdb); err != nil {
		log.Printf("redis tracing instrumentation: %v", err)
	}

	var dispatcher events.Dispatcher
	kdisp, err := events.NewKafkaDispatcher(cfg.Kafka.Brokers)
	if err != nil {
		log.Printf("kafka dispatcher unavailable, falling back to noop: %v", err)
		dispatcher = events.NewNoopDispatcher()
	} else {
		dispatcher = kdisp
	}

	likeRepo := NewLikeRepo(db)
	likes := NewCachedLikeStorage(likeRepo, rdb)

	commentRepo := NewCommentRepo(db)
	bookmarkRepo := NewBookmarkRepo(db)
	svc := NewInteractionService(likes, commentRepo, bookmarkRepo, dispatcher)

	lis, err := net.Listen("tcp", ":9005")
	if err != nil {
		log.Fatalf("listen: %v", err)
	}

	s := grpc.NewServer(telemetry.GRPCServerOptions(telemetry.StatsHandler())...)
	pb.RegisterInteractionServiceServer(s, svc)

	log.Printf("interaction service listening on :9005")
	if err := s.Serve(lis); err != nil {
		log.Fatalf("serve: %v", err)
	}
}
