package events

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/segmentio/kafka-go"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/propagation"
	"go.opentelemetry.io/otel/trace"
)

// KafkaDispatcher publishes events to Kafka using the pure-Go segmentio/kafka-go
// client. Each Dispatch creates an OpenTelemetry span so Kafka produce operations
// appear in distributed traces (Jaeger). It lazily creates one writer per topic.
type KafkaDispatcher struct {
	brokers   []string
	tracer    trace.Tracer
	propagator propagation.TextMapPropagator
	mu        sync.Mutex
	writers   map[string]*kafka.Writer
}

// NewKafkaDispatcher creates a Kafka-backed event dispatcher with OpenTelemetry tracing.
// brokers is a list of Kafka bootstrap servers, e.g. ["localhost:9092"].
func NewKafkaDispatcher(brokers []string) (*KafkaDispatcher, error) {
	d := &KafkaDispatcher{
		brokers:    brokers,
		tracer:     otel.Tracer("github.com/TheChosenGay/feeds/pkg/events"),
		propagator: otel.GetTextMapPropagator(),
		writers:    make(map[string]*kafka.Writer),
	}
	// Test connectivity by dialing the controller.
	conn, err := kafka.Dial("tcp", brokers[0])
	if err != nil {
		return nil, fmt.Errorf("kafka dispatcher: dial %s: %w", brokers[0], err)
	}
	conn.Close()
	log.Printf("[events] kafka dispatcher connected to %v", brokers)
	return d, nil
}

// Dispatch publishes an event to Kafka. It creates an OpenTelemetry span for the
// produce operation and injects trace context into Kafka headers so downstream
// consumers can continue the trace. Errors are logged but never returned —
// event publishing is fire-and-forget.
func (d *KafkaDispatcher) Dispatch(ctx context.Context, event Event) error {
	// Start a span for the Kafka produce.
	ctx, span := d.tracer.Start(ctx, "kafka.produce",
		trace.WithSpanKind(trace.SpanKindProducer),
		trace.WithAttributes(
			attribute.String("messaging.system", "kafka"),
			attribute.String("messaging.destination", event.Topic),
			attribute.String("messaging.kafka.message.key", event.Key),
		),
	)
	defer span.End()

	w := d.writerFor(event.Topic)

	// Inject trace context into Kafka headers.
	headers := make([]kafka.Header, 0)
	carrier := propagation.MapCarrier{}
	d.propagator.Inject(ctx, carrier)
	for k, v := range carrier {
		headers = append(headers, kafka.Header{Key: k, Value: []byte(v)})
	}

	// Use a short timeout so we don't block the caller if Kafka is slow.
	writeCtx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()

	err := w.WriteMessages(writeCtx, kafka.Message{
		Key:     []byte(event.Key),
		Value:   event.Body,
		Headers: headers,
	})
	if err != nil {
		log.Printf("[events] kafka write error: topic=%s key=%s err=%v", event.Topic, event.Key, err)
		span.SetStatus(codes.Error, err.Error())
	} else {
		span.SetStatus(codes.Ok, "")
	}
	return nil // fire-and-forget
}

// writerFor returns a topic-scoped writer, creating one lazily if needed.
func (d *KafkaDispatcher) writerFor(topic string) *kafka.Writer {
	d.mu.Lock()
	defer d.mu.Unlock()

	if w, ok := d.writers[topic]; ok {
		return w
	}

	w := &kafka.Writer{
		Addr:                   kafka.TCP(d.brokers...),
		Topic:                  topic,
		Balancer:               &kafka.LeastBytes{},
		BatchSize:              100,
		BatchTimeout:           5 * time.Millisecond,
		RequiredAcks:           kafka.RequireOne,
		Compression:            0, // no compression — small JSON payloads
		AllowAutoTopicCreation: true,
	}
	d.writers[topic] = w
	return w
}

// Close flushes and shuts down all writers.
func (d *KafkaDispatcher) Close() {
	d.mu.Lock()
	defer d.mu.Unlock()

	for topic, w := range d.writers {
		if err := w.Close(); err != nil {
			log.Printf("[events] kafka close writer for %s: %v", topic, err)
		}
	}
	d.writers = nil
}
