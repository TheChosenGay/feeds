package comet

import (
	"bytes"
	"context"
	"log/slog"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/TheChosenGay/feeds/pkg/comet"

// Server 是协议无关的 comet server 接口。
type Server interface {
	Start(ctx context.Context) error
	Push(roomID string, data []byte) int
	ConnCount() int
}

// Pusher 消息推送能力。Core 实现此接口。
// 业务层通过此接口推送消息，避免直接依赖 *Core。
type Pusher interface {
	Push(roomID string, data []byte) int
}

// ServerConfig 创建 Core 的依赖注入。
type ServerConfig struct {
	Business    Business
	ConnManager *ConnManager
}

// ============================================================
// Core — 协议无关的共享 dispatch 逻辑
// ============================================================

// Core 封装了消息分发、鉴权、心跳、推送等协议无关逻辑。
// 各协议实现（ws.Server / tcp.Server）嵌入 *Core 复用。
type Core struct {
	cfg    ServerConfig
	tracer trace.Tracer
	logger *slog.Logger
}

// NewCore 创建共享 dispatch 核心。
func NewCore(cfg ServerConfig) *Core {
	if cfg.ConnManager == nil {
		cfg.ConnManager = NewConnManager()
	}
	return &Core{
		cfg:    cfg,
		tracer: otel.Tracer(tracerName),
		logger: slog.With("component", "comet"),
	}
}

// ConnManager 返回内部的 ConnManager。
func (c *Core) ConnManager() *ConnManager {
	return c.cfg.ConnManager
}

// Push 向指定房间广播消息。
func (c *Core) Push(roomID string, data []byte) int {
	ctx, span := c.tracer.Start(context.Background(), "server.push",
		trace.WithAttributes(attribute.String("room_id", roomID)))
	defer span.End()

	frame := append(TypeMessage[:], data...)
	delivered := c.cfg.ConnManager.PushToRoom(roomID, frame)
	span.SetAttributes(attribute.Int("delivered", delivered))
	_ = ctx
	return delivered
}

// ConnCount 返回当前连接总数。
func (c *Core) ConnCount() int {
	return c.cfg.ConnManager.ConnCount()
}

// Dispatch 消息分发。协议实现的 onRead 回调应调用此方法。
func (c *Core) Dispatch(ctx context.Context, conn Conn, raw []byte) {
	if len(raw) < FrameHeaderSize {
		c.logger.Warn("frame too short", "len", len(raw), "conn_id", conn.ID())
		return
	}

	header := [2]byte(raw[0:2])
	payload := raw[FrameHeaderSize:]

	switch {
	case bytes.Equal(header[:], TypeHeartbeat[:]):
		c.handleHeartbeat(ctx, conn)

	case bytes.Equal(header[:], TypeAuth[:]):
		ctx, span := c.tracer.Start(ctx, "ws.auth",
			trace.WithAttributes(attribute.String("conn_id", conn.ID())))
		defer span.End()
		c.handleAuth(ctx, conn, payload)

	case bytes.Equal(header[:], TypeMessage[:]):
		ctx, span := c.tracer.Start(ctx, "ws.message",
			trace.WithAttributes(
				attribute.String("conn_id", conn.ID()),
				attribute.Int("payload_size", len(payload)),
			))
		defer span.End()
		c.handleMessage(ctx, conn, payload)

	default:
		c.logger.Warn("unknown message type", "header", header, "conn_id", conn.ID())
	}
}

// ============================================================
// 内部方法
// ============================================================

func (c *Core) handleHeartbeat(ctx context.Context, conn Conn) {
	if err := conn.Write(ctx, HeartbeatReply[:]); err != nil {
		c.logger.Warn("heartbeat write failed", "conn_id", conn.ID(), "err", err)
	}
}

func (c *Core) handleAuth(ctx context.Context, conn Conn, payload []byte) {
	userID, err := c.cfg.Business.OnAuth(ctx, payload)
	if err != nil || userID == "" {
		c.logger.Warn("auth failed", "conn_id", conn.ID(), "err", err)
		span := trace.SpanFromContext(ctx)
		span.SetStatus(codes.Error, "auth failed")
		if err != nil {
			span.RecordError(err)
		}
		conn.Write(ctx, []byte{0x00, 0x00})
		return
	}

	// 绑定到房间（roomID = userID）
	c.cfg.ConnManager.Bind(userID, conn)

	trace.SpanFromContext(ctx).SetAttributes(attribute.String("user_id", userID))
	c.logger.Info("auth success", "conn_id", conn.ID(), "user_id", userID)
	conn.Write(ctx, []byte{0x00, 0x01})
}

func (c *Core) handleMessage(ctx context.Context, conn Conn, payload []byte) {
	userID := c.cfg.ConnManager.RoomOf(conn.ID())
	if userID == "" {
		c.logger.Warn("message from unauthenticated conn", "conn_id", conn.ID())
		trace.SpanFromContext(ctx).SetStatus(codes.Error, "unauthenticated")
		return
	}

	span := trace.SpanFromContext(ctx)
	span.SetAttributes(attribute.String("user_id", userID))

	if err := c.cfg.Business.OnMessage(ctx, conn.ID(), userID, payload); err != nil {
		c.logger.Warn("onMessage error", "conn_id", conn.ID(), "user_id", userID, "err", err)
		span.SetStatus(codes.Error, "onMessage failed")
		span.RecordError(err)
	}
}
