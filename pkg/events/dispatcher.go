package events

import (
	"context"
	"log"
)

// Event represents a domain event to be published asynchronously.
type Event struct {
	Topic string
	Key   string
	Body  []byte
}

// Dispatcher publishes events to a message queue.
// Implementations: noop (dev/test), Kafka, Pulsar, etc.
type Dispatcher interface {
	Dispatch(ctx context.Context, event Event) error
}

// --- Noop dispatcher (dev / testing) ---

type noopDispatcher struct{}

// NewNoopDispatcher returns a dispatcher that logs events without publishing.
func NewNoopDispatcher() Dispatcher {
	return &noopDispatcher{}
}

func (d *noopDispatcher) Dispatch(ctx context.Context, event Event) error {
	log.Printf("[events] noop: topic=%s key=%s", event.Topic, event.Key)
	return nil
}
