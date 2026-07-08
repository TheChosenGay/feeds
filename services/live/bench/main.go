// comet-bench — comet 长连接压测工具
//
// 用法:
//
//	go run ./services/comet/bench
//	go run ./services/comet/bench -conns 10000 -duration 30s -rate 100
//
// 系统准备:
//
//	ulimit -n 10240   # macOS/Linux，大连接数必须调高 fd 上限
package main

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/gorilla/websocket"
)

// ============================================================
// CLI flags
// ============================================================

var (
	addr     = flag.String("addr", "ws://localhost:8081/ws", "comet WebSocket address")
	nConns   = flag.Int("conns", 5000, "number of concurrent connections")
	duration = flag.Duration("duration", 20*time.Second, "test duration")
	rate     = flag.Int("rate", 50, "messages per second per connection")
	payload  = flag.Int("payload", 64, "message payload size in bytes")
	batch    = flag.Int("batch", 200, "connection establishment concurrency")
)

// ============================================================
// JWT helpers
// ============================================================

const jwtSecret = "feeds-dev-secret"

func sign(data []byte) []byte {
	mac := hmac.New(sha256.New, []byte(jwtSecret))
	mac.Write(data)
	return mac.Sum(nil)
}

func makeToken(userID string) string {
	header := base64.RawURLEncoding.EncodeToString([]byte(`{"alg":"HS256","typ":"JWT"}`))
	claims := fmt.Sprintf(`{"user_id":"%s","iat":%d}`, userID, time.Now().Unix())
	payload := base64.RawURLEncoding.EncodeToString([]byte(claims))
	sig := base64.RawURLEncoding.EncodeToString(sign([]byte(header + "." + payload)))
	return header + "." + payload + "." + sig
}

// ============================================================
// Frame helpers
// ============================================================

var (
	ftAuth    = [2]byte{0x00, 0x02}
	ftMessage = [2]byte{0x00, 0x03}
)

// ============================================================
// Metrics
// ============================================================

type latencyTracker struct {
	mu    sync.Mutex
	times []time.Duration
}

func (lt *latencyTracker) add(d time.Duration) {
	lt.mu.Lock()
	lt.times = append(lt.times, d)
	lt.mu.Unlock()
}

func (lt *latencyTracker) percentiles() (p50, p95, p99, max, avg time.Duration) {
	lt.mu.Lock()
	defer lt.mu.Unlock()

	if len(lt.times) == 0 {
		return
	}

	sorted := make([]time.Duration, len(lt.times))
	copy(sorted, lt.times)
	slices.Sort(sorted)

	var total time.Duration
	for _, d := range sorted {
		total += d
		if d > max {
			max = d
		}
	}

	avg = total / time.Duration(len(sorted))
	p50 = sorted[len(sorted)*50/100]
	p95 = sorted[len(sorted)*95/100]
	p99 = sorted[len(sorted)*99/100]
	return
}

// ============================================================
// Main
// ============================================================

func main() {
	flag.Parse()

	expectedRate := *nConns * *rate

	log.Printf("=== comet bench ===")
	log.Printf("addr:      %s", *addr)
	log.Printf("conns:     %d (batch %d)", *nConns, *batch)
	log.Printf("duration:  %s", *duration)
	log.Printf("rate:      %d msg/s/conn → ~%d msg/s total", *rate, expectedRate)
	log.Printf("payload:   %d bytes", *payload)
	log.Println()

	if *nConns > 5000 {
		log.Printf("⚠️  high connection count (%d) — check fd limit:", *nConns)
		log.Printf("   ulimit -n %d", *nConns+1000)
		log.Println()
	}

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	// ---- Phase 1: Connect + Auth ----
	log.Println("--- Phase 1: Connect + Auth ---")

	var (
		conns       []*websocket.Conn
		connsMu     sync.Mutex
		authLatency = &latencyTracker{}
		connOK      atomic.Int64
		connFail    atomic.Int64
	)

	t0 := time.Now()
	var wg sync.WaitGroup
	sem := make(chan struct{}, *batch)

	for i := 0; i < *nConns; i++ {
		wg.Add(1)
		sem <- struct{}{}
		go func(idx int) {
			defer wg.Done()
			defer func() { <-sem }()

			token := makeToken(fmt.Sprintf("bench-%d", idx))
			dialStart := time.Now()

			ws, _, err := websocket.DefaultDialer.DialContext(ctx, *addr, nil)
			if err != nil {
				connFail.Add(1)
				if connFail.Load() <= 5 {
					log.Printf("[%d] dial failed: %v", idx, err)
				}
				return
			}

			frame := append(ftAuth[:], []byte(token)...)
			if err := ws.WriteMessage(websocket.BinaryMessage, frame); err != nil {
				connFail.Add(1)
				ws.Close()
				return
			}

			ws.SetReadDeadline(time.Now().Add(5 * time.Second))
			_, raw, err := ws.ReadMessage()
			if err != nil || len(raw) < 2 || raw[0] != 0x00 || raw[1] != 0x01 {
				connFail.Add(1)
				ws.Close()
				return
			}

			authLatency.add(time.Since(dialStart))
			connOK.Add(1)

			connsMu.Lock()
			conns = append(conns, ws)
			connsMu.Unlock()
		}(i)

		// 每 500 条输出一次进度
		if i > 0 && i%500 == 0 {
			log.Printf("  established %d/%d (%.0f%%) ...", connOK.Load(), *nConns,
				float64(connOK.Load())/float64(*nConns)*100)
		}
	}
	wg.Wait()

	connectTime := time.Since(t0)
	p50a, p95a, p99a, maxA, avgA := authLatency.percentiles()

	log.Printf("connections: %d ok / %d fail (%.1f%%)",
		connOK.Load(), connFail.Load(),
		float64(connOK.Load())/float64(*nConns)*100)
	log.Printf("ramp-up:   %s (%.0f conn/s)", connectTime,
		float64(connOK.Load())/connectTime.Seconds())
	log.Printf("auth latency: avg=%s p50=%s p95=%s p99=%s max=%s",
		avgA, p50a, p95a, p99a, maxA)
	log.Println()

	if connOK.Load() == 0 {
		log.Fatal("no connections established, aborting")
	}

	// ---- Phase 2: Message throughput ----
	log.Printf("--- Phase 2: Message Throughput (target %d msg/s) ---", expectedRate)

	var (
		msgSent    atomic.Int64
		msgErr     atomic.Int64
		msgLatency = &latencyTracker{}
		done       atomic.Bool
		body       = []byte(strings.Repeat("x", *payload))
	)

	nOK := int(connOK.Load())
	for i := 0; i < nOK; i++ {
		ws := conns[i]
		wg.Add(1)
		go func(connIdx int, conn *websocket.Conn) {
			defer wg.Done()

			interval := time.Second / time.Duration(*rate)
			ticker := time.NewTicker(interval)
			defer ticker.Stop()

			// send loop
			sendDone := make(chan struct{})
			go func() {
				defer close(sendDone)
				for !done.Load() {
					<-ticker.C
					frame := append(ftMessage[:], body...)
					t0 := time.Now()
					if err := conn.WriteMessage(websocket.BinaryMessage, frame); err != nil {
						msgErr.Add(1)
						if msgErr.Load() <= 10 {
							log.Printf("[send-err] conn=%d: %v", connIdx, err)
						}
					} else {
						msgSent.Add(1)
						msgLatency.add(time.Since(t0))
					}
				}
			}()

			// recv loop (handle heartbeat)
			for !done.Load() {
				conn.SetReadDeadline(time.Now().Add(120 * time.Second))
				_, raw, err := conn.ReadMessage()
				if err != nil {
					if !done.Load() {
						msgErr.Add(1)
						if msgErr.Load() <= 10 {
							log.Printf("[recv-err] conn=%d: %v", connIdx, err)
						}
					}
					return
				}
				if len(raw) >= 2 && raw[0] == 0x00 && raw[1] == 0x01 {
					conn.WriteMessage(websocket.BinaryMessage, []byte{0x00, 0x00})
				}
			}
			<-sendDone
		}(i, ws)
	}

	// Progress reporting every 3 seconds
	startTime := time.Now()
	go func() {
		ticker := time.NewTicker(3 * time.Second)
		defer ticker.Stop()
		var lastSent int64
		for range ticker.C {
			if done.Load() {
				return
			}
			elapsed := time.Since(startTime)
			sent := msgSent.Load()
			errors := msgErr.Load()
			instant := float64(sent-lastSent) / 3.0
			lastSent = sent
			log.Printf("[%s] sent=%d rate=%.0f/s errors=%d conns=%d",
				elapsed.Round(time.Second), sent, instant, errors, nOK)
		}
	}()

	select {
	case <-ctx.Done():
		log.Println("interrupted")
	case <-time.After(*duration):
	}
	done.Store(true)

	// 先关闭所有连接，让阻塞在 ReadMessage 的 recv goroutine 退出
	for _, ws := range conns {
		ws.Close()
	}
	wg.Wait()

	// ---- Report ----
	elapsed := time.Since(startTime)
	sent := msgSent.Load()
	errors := msgErr.Load()
	p50m, p95m, p99m, maxM, avgM := msgLatency.percentiles()

	fmt.Println()
	fmt.Println(strings.Repeat("=", 60))
	fmt.Println("  COMET BENCH REPORT")
	fmt.Println(strings.Repeat("=", 60))
	fmt.Printf("  connections:   %d ok / %d fail\n", connOK.Load(), connFail.Load())
	fmt.Printf("  ramp-up:       %s\n", connectTime.Round(time.Millisecond))
	fmt.Printf("  test duration: %s\n", elapsed.Round(time.Millisecond))
	fmt.Printf("  messages:      %d sent / %d errors (%.4f%%)\n",
		sent, errors, float64(errors)/float64(max(sent, 1))*100)
	if elapsed.Seconds() > 0 {
		fmt.Printf("  throughput:    %.0f msg/s (target %d)\n", float64(sent)/elapsed.Seconds(), expectedRate)
	}
	fmt.Println()
	fmt.Println("  --- auth handshake latency ---")
	fmt.Printf("  avg:  %s  p50:  %s  p95:  %s  p99:  %s  max:  %s\n",
		avgA, p50a, p95a, p99a, maxA)
	fmt.Println()
	fmt.Println("  --- write latency ---")
	fmt.Printf("  avg:  %s  p50:  %s  p95:  %s  p99:  %s  max:  %s\n",
		avgM, p50m, p95m, p99m, maxM)
	fmt.Println(strings.Repeat("=", 60))

	if errors > 0 && float64(errors)/float64(max(sent, 1)) > 0.01 {
		fmt.Printf("FAIL: error rate %.2f%% > 1%%\n", float64(errors)/float64(max(sent, 1))*100)
		os.Exit(1)
	}
	if p95m > 500*time.Millisecond {
		fmt.Printf("FAIL: p95 write latency %s > 500ms\n", p95m)
		os.Exit(1)
	}
	if p99m > time.Second {
		fmt.Printf("FAIL: p99 write latency %s > 1s\n", p99m)
		os.Exit(1)
	}

	fmt.Printf("OK: %.0f msg/s, p95=%s\n", float64(sent)/max(elapsed, time.Second).Seconds(), p95m)
}
