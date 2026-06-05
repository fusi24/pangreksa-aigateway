// Package producer provides a fire-and-forget Kafka producer for TransactionEvents.
// The hot path (PublishAsync) enqueues events on a buffered internal channel and
// returns immediately. A background goroutine drains the channel and writes to Kafka
// with retry/back-off so caller latency is never affected by Kafka throughput.
//
// Topic:         llm.transactions
// Partition key: org_id
package producer

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/segmentio/kafka-go"

	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

const (
	// queueDepth is the size of the internal event channel.
	// Overflow events are dropped and a warning is logged.
	queueDepth = 4096
	// maxRetries is the number of Kafka write attempts before dropping an event.
	maxRetries = 3
	// retryDelay is the initial delay between retries; doubles on each retry.
	retryDelay = 500 * time.Millisecond
)

// Producer publishes TransactionEvents to Kafka asynchronously.
// The hot path returns immediately after enqueuing; a background goroutine handles
// actual publish with retry/back-off.
//
// Thread-safety: safe for concurrent use; PublishAsync is goroutine-safe.
type Producer struct {
	// writer is the kafka-go writer for the llm.transactions topic.
	writer *kafka.Writer
	// queue is the internal channel for decoupling callers from Kafka writes.
	queue chan model.TransactionEvent
	// log is the structured logger.
	log zerolog.Logger
}

// NewProducer constructs a Producer and validates that at least one broker address is supplied.
// It does not establish a connection at construction time; the connection is made lazily.
//
// brokers is a comma-separated list of Kafka broker addresses.
// topic is the Kafka topic name (should be "llm.transactions").
func NewProducer(brokers, topic string, log zerolog.Logger) (*Producer, error) {
	if strings.TrimSpace(brokers) == "" {
		return nil, fmt.Errorf("producer.NewProducer: brokers must not be empty")
	}
	if strings.TrimSpace(topic) == "" {
		return nil, fmt.Errorf("producer.NewProducer: topic must not be empty")
	}

	brokerList := parseBrokers(brokers)

	writer := &kafka.Writer{
		Addr:                   kafka.TCP(brokerList...),
		Topic:                  topic,
		Balancer:               &kafka.Hash{}, // hash on message Key (org_id) for sticky partitioning
		RequiredAcks:           kafka.RequireOne,
		Async:                  false, // synchronous within our retry loop
		AllowAutoTopicCreation: false,
	}

	return &Producer{
		writer: writer,
		queue:  make(chan model.TransactionEvent, queueDepth),
		log:    log.With().Str("component", "kafka-producer").Logger(),
	}, nil
}

// Start begins the background publish goroutine that drains the event queue.
// It must be called exactly once before PublishAsync.
// The goroutine exits when ctx is cancelled; Close should still be called to flush.
func (p *Producer) Start(ctx context.Context) {
	go func() {
		defer func() {
			if r := recover(); r != nil {
				p.log.Error().Interface("panic", r).Msg("producer goroutine: recovered from panic")
			}
		}()

		p.log.Info().Msg("kafka producer background goroutine started")

		for {
			select {
			case <-ctx.Done():
				p.log.Info().Msg("kafka producer goroutine stopping; draining queue")
				p.drain()
				return
			case event := <-p.queue:
				p.publish(ctx, event)
			}
		}
	}()
}

// PublishAsync enqueues a TransactionEvent for asynchronous Kafka publish.
// It never blocks the caller; if the internal queue is full the event is dropped
// and a warning is logged.
func (p *Producer) PublishAsync(event model.TransactionEvent) {
	select {
	case p.queue <- event:
	default:
		p.log.Warn().
			Str("event_id", event.EventID).
			Str("request_id", event.RequestID).
			Msg("producer queue full; dropping event")
	}
}

// Close drains the pending event queue and closes the Kafka writer.
// Should be called during graceful shutdown after cancelling the context passed to Start.
func (p *Producer) Close() error {
	p.drain()
	if err := p.writer.Close(); err != nil {
		return fmt.Errorf("producer.Close: %w", err)
	}
	return nil
}

// publish writes a single TransactionEvent to Kafka with retry/back-off.
func (p *Producer) publish(ctx context.Context, event model.TransactionEvent) {
	data, err := json.Marshal(event)
	if err != nil {
		p.log.Error().Err(err).Str("event_id", event.EventID).Msg("failed to marshal event; dropping")
		return
	}

	msg := kafka.Message{
		Key:   []byte(event.OrgID), // partition by org_id for locality
		Value: data,
	}

	delay := retryDelay
	for attempt := 1; attempt <= maxRetries; attempt++ {
		writeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		err := p.writer.WriteMessages(writeCtx, msg)
		cancel()

		if err == nil {
			p.log.Debug().Str("event_id", event.EventID).Msg("event published")
			return
		}

		p.log.Warn().Err(err).
			Int("attempt", attempt).
			Str("event_id", event.EventID).
			Msg("kafka write failed; retrying")

		if attempt < maxRetries {
			select {
			case <-ctx.Done():
				return
			case <-time.After(delay):
			}
			delay *= 2
		}
	}

	p.log.Error().
		Str("event_id", event.EventID).
		Int("max_retries", maxRetries).
		Msg("kafka write failed after all retries; dropping event")
}

// drain processes all events remaining in the queue synchronously.
// Called during shutdown to minimise event loss.
func (p *Producer) drain() {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	for {
		select {
		case event := <-p.queue:
			p.publish(ctx, event)
		case <-ctx.Done():
			return
		default:
			return
		}
	}
}

// parseBrokers splits a comma-separated broker string into a slice.
func parseBrokers(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
