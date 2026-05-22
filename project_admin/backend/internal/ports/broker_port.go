package ports

import "context"

// BrokerPort is the message-broker surface for async work. v1 backed by
// Redis Streams; could be swapped for NATS / Postgres-based queue / etc.
//
// Semantics:
//   - Publish is fire-and-forget. Delivery durability comes from the
//     broker's persistence layer (Redis AOF for Streams).
//   - Subscribe blocks until ctx is cancelled. It dispatches messages
//     to handler one at a time per consumer; multiple consumers in the
//     same group share work via the broker's load balancing.
//   - When handler returns nil → message is ACKed. When handler returns
//     non-nil → message stays in pending list and will be redelivered
//     to another consumer (Redis XAUTOCLAIM on idle).
//
// One-message-at-a-time per consumer is the v1 contract. If we need
// higher throughput per worker we add a COUNT param to Subscribe.
type BrokerPort interface {
	Publish(ctx context.Context, topic string, payload []byte) error
	Subscribe(ctx context.Context, topic, group, consumer string,
		handler func(ctx context.Context, payload []byte) error) error
	Close() error
}
