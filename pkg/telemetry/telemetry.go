package telemetry

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/TheChosenGay/feeds/pkg/config"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/exporters/otlp/otlpmetric/otlpmetricgrpc"
	"go.opentelemetry.io/otel/exporters/otlp/otlptrace/otlptracegrpc"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/resource"
	"go.opentelemetry.io/otel/sdk/trace"
	semconv "go.opentelemetry.io/otel/semconv/v1.26.0"
	"google.golang.org/grpc"
	"google.golang.org/grpc/stats"

	"go.opentelemetry.io/contrib/instrumentation/google.golang.org/grpc/otelgrpc"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// Init 初始化 OpenTelemetry（traces + metrics）并启动 Prometheus metrics 端点。
// 返回 shutdown 函数，main 里 defer 调用确保数据 flush。
func Init(ctx context.Context, serviceName string) (func(context.Context) error, error) {
	// otel-collector endpoint
	endpoint := config.GetEnv("OTEL_EXPORTER_OTLP_ENDPOINT", "otel-collector:4317")

	res, err := resource.New(ctx,
		resource.WithAttributes(semconv.ServiceName(serviceName)),
	)
	if err != nil {
		return nil, fmt.Errorf("resource: %w", err)
	}

	// trace exporter
	traceExp, err := otlptracegrpc.New(ctx,
		otlptracegrpc.WithEndpoint(endpoint),
		otlptracegrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("trace exporter: %w", err)
	}

	tp := trace.NewTracerProvider(
		trace.WithBatcher(traceExp),
		trace.WithResource(res),
		trace.WithSampler(trace.AlwaysSample()),
	)
	otel.SetTracerProvider(tp)
	otel.SetTextMapPropagator(propagation.NewCompositeTextMapPropagator(
		propagation.TraceContext{}, propagation.Baggage{},
	))

	// metric exporter
	metricExp, err := otlpmetricgrpc.New(ctx,
		otlpmetricgrpc.WithEndpoint(endpoint),
		otlpmetricgrpc.WithInsecure(),
	)
	if err != nil {
		return nil, fmt.Errorf("metric exporter: %w", err)
	}

	mp := metric.NewMeterProvider(
		metric.WithReader(metric.NewPeriodicReader(metricExp, metric.WithInterval(15*time.Second))),
		metric.WithResource(res),
	)
	otel.SetMeterProvider(mp)

	// metrics HTTP 端点（Prometheus 抓取）
	go serveMetrics()

	shutdown := func(ctx context.Context) error {
		if e := tp.Shutdown(ctx); e != nil {
			return e
		}
		return mp.Shutdown(ctx)
	}

	log.Printf("[telemetry] %s ready, metrics :9090, otlp → %s", serviceName, endpoint)
	return shutdown, nil
}

// GRPCServerOptions 返回 gRPC server 需要的 option（stats handler）
func GRPCServerOptions(handler stats.Handler) []grpc.ServerOption {
	return []grpc.ServerOption{
		grpc.StatsHandler(handler),
	}
}

// StatsHandler 返回 OpenTelemetry 的 gRPC server stats handler
func StatsHandler() stats.Handler {
	return otelgrpc.NewServerHandler()
}

// ClientStatsHandler 返回 OpenTelemetry 的 gRPC client stats handler
func ClientStatsHandler() stats.Handler {
	return otelgrpc.NewClientHandler()
}

// HTTPMiddleware 用 otelhttp 包裹一个 HTTP handler
func HTTPMiddleware(handler http.Handler, name string) http.Handler {
	return otelhttp.NewHandler(handler, name)
}

func serveMetrics() {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	if err := http.ListenAndServe(":9090", mux); err != nil {
		log.Printf("[telemetry] metrics serve: %v", err)
	}
}
