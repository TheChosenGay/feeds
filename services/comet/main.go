package main

import (
	"context"
	"log"
	"net"
	"net/http"
	_ "net/http/pprof"
	"os/signal"
	"syscall"

	"github.com/TheChosenGay/feeds/pkg/auth"
	"github.com/TheChosenGay/feeds/pkg/comet"
	"github.com/TheChosenGay/feeds/pkg/comet/ws"
	"github.com/TheChosenGay/feeds/pkg/config"
	"github.com/TheChosenGay/feeds/pkg/telemetry"
	pb "github.com/TheChosenGay/feeds/proto/gen/comet"
	"google.golang.org/grpc"
	"google.golang.org/grpc/reflection"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	shutdown, err := telemetry.Init(context.Background(), "comet")
	if err != nil {
		log.Fatalf("telemetry: %v", err)
	}
	defer shutdown(context.Background())

	cfg := config.Load("comet")
	_ = cfg

	wsAddr := config.GetEnv("COMET_WS_ADDR", ":8081")
	grpcAddr := config.GetEnv("COMET_GRPC_ADDR", ":9006")

	// 1. 创建基础设施
	manager := comet.NewConnManager()

	// 2. 创建 Business + Core
	//    Business → Pusher 接口，Core → Business 接口，双向依赖接口，不会循环引用
	biz := &cometBusiness{}
	core := comet.NewCore(comet.ServerConfig{
		ConnManager: manager,
		Business:    biz,
	})
	biz.push = core // Pusher 后置注入，消息在 Start 之后才来，此时已就绪

	// 4. 创建 WS server（传入已配置好的 Core）
	wsSrv := ws.NewServerWithCore(wsAddr, core)

	// 启动
	go func() {
		if err := wsSrv.Start(ctx); err != nil {
			log.Fatalf("ws serve: %v", err)
		}
	}()

	// gRPC server
	grpcSrv := grpc.NewServer(telemetry.GRPCServerOptions(telemetry.StatsHandler())...)
	reflection.Register(grpcSrv)
	pb.RegisterCometServiceServer(grpcSrv, &cometGRPC{core: core})

	lis, err := net.Listen("tcp", grpcAddr)
	if err != nil {
		log.Fatalf("grpc listen: %v", err)
	}

	go func() {
		log.Printf("comet grpc listening on %s", grpcAddr)
		if err := grpcSrv.Serve(lis); err != nil {
			log.Fatalf("grpc serve: %v", err)
		}
	}()

	// pprof
	pprofAddr := config.GetEnv("COMET_PPROF_ADDR", ":6060")
	go func() {
		log.Printf("pprof listening on %s", pprofAddr)
		if err := http.ListenAndServe(pprofAddr, nil); err != nil {
			log.Printf("pprof: %v", err)
		}
	}()

	log.Printf("comet started: ws=%s grpc=%s", wsAddr, grpcAddr)

	<-ctx.Done()
	log.Println("comet shutting down...")
	grpcSrv.GracefulStop()
}

// cometBusiness 实现 comet.Business 接口。
// 通过 comet.Pusher 接口推送消息，避免循环依赖 *comet.Core。
type cometBusiness struct {
	push comet.Pusher
}

func (b *cometBusiness) OnAuth(ctx context.Context, payload []byte) (string, error) {
	return auth.ValidateToken(string(payload))
}

func (b *cometBusiness) OnMessage(ctx context.Context, connID string, userID string, payload []byte) error {
	// 业务消息处理。可通过 b.push.Push(roomID, data) 向任意用户推送。
	return nil
}

// cometGRPC 实现 CometServiceServer。
type cometGRPC struct {
	pb.UnimplementedCometServiceServer
	core *comet.Core
}

func (g *cometGRPC) PushRoom(ctx context.Context, req *pb.PushRoomReq) (*pb.PushResp, error) {
	delivered := g.core.Push(req.RoomId, req.Payload)
	return &pb.PushResp{Delivered: int32(delivered)}, nil
}

func (g *cometGRPC) PushUser(ctx context.Context, req *pb.PushUserReq) (*pb.PushResp, error) {
	delivered := g.core.Push(req.UserId, req.Payload)
	return &pb.PushResp{Delivered: int32(delivered)}, nil
}

func (g *cometGRPC) IsOnline(ctx context.Context, req *pb.OnlineReq) (*pb.OnlineResp, error) {
	online, count := g.core.RoomOnline(req.UserId)
	return &pb.OnlineResp{Online: online, DeviceCount: int32(count)}, nil
}
