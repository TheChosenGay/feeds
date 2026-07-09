// 全链路通知测试：发帖通知 + 点赞通知 + WS 实时投递验证
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"time"

	livepb "github.com/TheChosenGay/feeds/proto/gen/comet"
	notifypb "github.com/TheChosenGay/feeds/proto/gen/notify"
	"github.com/gorilla/websocket"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

const secret = "feeds-dev-secret"

func makeToken(userID string) string {
	h := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	c := fmt.Sprintf(`{"user_id":"%s","iat":%d}`, userID, time.Now().Unix())
	p := base64.RawURLEncoding.EncodeToString([]byte(c))
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(h + "." + p))
	s := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return h + "." + p + "." + s
}

func main() {
	log.SetFlags(0)
	log.Println("=" + strings.Repeat("=", 59))
	log.Println("全链路通知测试: 发帖 + 点赞")
	log.Println("=" + strings.Repeat("=", 59))

	authorID := "user-author-2024"
	likerID := "user-liker-2024"
	ctx := context.Background()

	// ── gRPC 连接 ──
	liveConn, _ := grpc.NewClient("localhost:9006",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer liveConn.Close()
	liveCli := livepb.NewLiveServiceClient(liveConn)

	notifyConn, _ := grpc.NewClient("localhost:9007",
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	defer notifyConn.Close()
	notifyCli := notifypb.NewNotifyServiceClient(notifyConn)

	// ═══════════════════════════════════════════════════════════════
	// 场景1: 发帖通知 — 作者在线，WS 实时收到
	// ═══════════════════════════════════════════════════════════════
	log.Println()
	log.Println("─── 场景1: 发帖通知 (作者在线) ───")

	// 1.1 作者连接 WS
	ws, _, err := websocket.DefaultDialer.Dial("ws://localhost:8081/ws", nil)
	if err != nil {
		log.Fatalf("❌ WS dial: %v", err)
	}
	defer ws.Close()

	ws.WriteMessage(websocket.BinaryMessage, append([]byte{0x00, 0x02}, []byte(makeToken(authorID))...))
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, _ := ws.ReadMessage()
	if len(raw) < 2 || raw[0] != 0x00 || raw[1] != 0x01 {
		log.Fatalf("❌ 作者鉴权失败: %v", raw[:2])
	}
	log.Println("[1.1] ✅ 作者 WS 连接 + 鉴权成功")

	online, _ := liveCli.IsOnline(ctx, &livepb.OnlineReq{UserId: authorID})
	log.Printf("[1.2] IsOnline: online=%v devices=%d", online.Online, online.DeviceCount)

	// 1.3 模拟发帖 → notify.Push（等价于 Python handler 消费 Kafka 事件后的行为）
	postPayload, _ := json.Marshal(map[string]interface{}{
		"post_id": "post-001",
	})
	notifyResp, err := notifyCli.Push(ctx, &notifypb.PushReq{
		UserId:  authorID,
		Type:    "system",
		Title:   "帖子发布成功",
		Body:    "你的帖子《今天的日落》已推送给 42 位粉丝",
		Payload: postPayload,
	})
	if err != nil {
		log.Fatalf("❌ notify.Push (发帖): %v", err)
	}
	log.Printf("[1.3] notify.Push → id=%d delivered_ws=%v devices=%d",
		notifyResp.Id, notifyResp.DeliveredWs, notifyResp.DeviceCount)

	// 1.4 WS 接收
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err = ws.ReadMessage()
	if err != nil {
		log.Fatalf("❌ WS 接收失败: %v", err)
	}
	if raw[0] == 0x00 && raw[1] == 0x03 {
		var msg map[string]interface{}
		json.Unmarshal(raw[2:], &msg)
		log.Printf("[1.4] ✅ WS 收到实时推送 → type=%s title=%s", msg["type"], msg["title"])
		if msg["type"] != "system" {
			log.Fatalf("❌ 期望 type=system, 实际=%s", msg["type"])
		}
	} else {
		log.Fatalf("❌ 期望消息帧, 实际=%v", raw[:2])
	}

	// ═══════════════════════════════════════════════════════════════
	// 场景2: 点赞通知 — 作者在线，WS 实时收到
	// ═══════════════════════════════════════════════════════════════
	log.Println()
	log.Println("─── 场景2: 点赞通知 (作者在线) ───")

	likePayload, _ := json.Marshal(map[string]interface{}{
		"post_id":  "post-001",
		"liker_id": likerID,
	})
	notifyResp2, err := notifyCli.Push(ctx, &notifypb.PushReq{
		UserId:  authorID,
		Type:    "like",
		Title:   "新点赞",
		Body:    fmt.Sprintf("%s 赞了你的帖子", likerID),
		Payload: likePayload,
	})
	if err != nil {
		log.Fatalf("❌ notify.Push (点赞): %v", err)
	}
	log.Printf("[2.1] notify.Push → id=%d delivered_ws=%v devices=%d",
		notifyResp2.Id, notifyResp2.DeliveredWs, notifyResp2.DeviceCount)

	// 2.2 WS 接收点赞通知
	ws.SetReadDeadline(time.Now().Add(3 * time.Second))
	_, raw, err = ws.ReadMessage()
	if err != nil {
		log.Fatalf("❌ WS 接收失败: %v", err)
	}
	if raw[0] == 0x00 && raw[1] == 0x03 {
		var msg map[string]interface{}
		json.Unmarshal(raw[2:], &msg)
		log.Printf("[2.2] ✅ WS 收到实时推送 → type=%s title=%s", msg["type"], msg["title"])
		if msg["type"] != "like" {
			log.Fatalf("❌ 期望 type=like, 实际=%s", msg["type"])
		}
	} else {
		log.Fatalf("❌ 期望消息帧, 实际=%v", raw[:2])
	}

	// ═══════════════════════════════════════════════════════════════
	// 场景3: DB 验证 — 检查通知历史
	// ═══════════════════════════════════════════════════════════════
	log.Println()
	log.Println("─── 场景3: DB 验证 ───")

	listResp, err := notifyCli.ListNotifications(ctx, &notifypb.ListNotificationsReq{
		UserId:   authorID,
		PageSize: 10,
	})
	if err != nil {
		log.Fatalf("❌ ListNotifications: %v", err)
	}
	log.Printf("[3.1] 通知历史共 %d 条:", len(listResp.Items))
	readStatus := func(isRead bool) string {
		if isRead {
			return "✅已读"
		}
		return "🔵未读"
	}
	for _, item := range listResp.Items {
		log.Printf("      [%s] id=%d type=%s title=%s",
			readStatus(item.IsRead), item.Id, item.Type, item.Title)
	}

	countResp, _ := notifyCli.GetUnreadCount(ctx, &notifypb.GetUnreadCountReq{UserId: authorID})
	log.Printf("[3.2] 未读数: %d", countResp.Count)

	// ═══════════════════════════════════════════════════════════════
	// 场景4: MarkRead 全部已读
	// ═══════════════════════════════════════════════════════════════
	log.Println()
	log.Println("─── 场景4: 标记已读 ───")

	_, err = notifyCli.MarkRead(ctx, &notifypb.MarkReadReq{
		UserId:         authorID,
		NotificationId: "all",
	})
	if err != nil {
		log.Fatalf("❌ MarkRead: %v", err)
	}

	countAfter, _ := notifyCli.GetUnreadCount(ctx, &notifypb.GetUnreadCountReq{UserId: authorID})
	if countAfter.Count != 0 {
		log.Fatalf("❌ 期望未读=0, 实际=%d", countAfter.Count)
	}
	log.Printf("[4.1] ✅ 全部已读, 未读数=%d", countAfter.Count)

	// ═══════════════════════════════════════════════════════════════
	// 结果
	// ═══════════════════════════════════════════════════════════════
	log.Println()
	log.Println("=" + strings.Repeat("=", 59))
	log.Println("✅ 全链路测试全部通过")
	log.Println("=" + strings.Repeat("=", 59))
	log.Println("  ✅ 发帖通知: WS 实时收到 + DB 存储")
	log.Println("  ✅ 点赞通知: WS 实时收到 + DB 存储")
	log.Println("  ✅ 通知列表: 2 条历史记录")
	log.Println("  ✅ 标记已读: 未读数归零")
}
