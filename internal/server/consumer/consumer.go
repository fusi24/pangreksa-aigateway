// Package consumer provides a Kafka consumer that reads TransactionEvents from
// the "llm.transactions" topic and persists them to PostgreSQL.
//
// Delivery guarantee: at-least-once.
// Idempotency: request_logs and cost_records use ON CONFLICT DO NOTHING so
// duplicate deliveries from Kafka are harmless.
//
// The consumer commits offsets only after both inserts succeed, ensuring no
// events are lost if the process crashes mid-way.
package consumer

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/rs/zerolog"
	kafka "github.com/segmentio/kafka-go"

	"github.com/pangreksa/ai-gateway-engine/internal/server/repository"
	"github.com/pangreksa/ai-gateway-engine/internal/server/sse"
	"github.com/pangreksa/ai-gateway-engine/pkg/model"
)

const (
	// defaultGroupID is the Kafka consumer group ID for the Central Server.
	defaultGroupID = "central-server-consumer"

	// readTimeout is the maximum time to wait for a new Kafka message before
	// looping back to check ctx.Done().
	readTimeout = 5 * time.Second
)

// Consumer reads TransactionEvents from Kafka and persists them to PostgreSQL.
// Delivery guarantee: at-least-once with idempotent inserts.
//
// Thread-safety: Start must be called from a single goroutine. The consumer
// is not safe for concurrent Start calls.
type Consumer struct {
	// reader is the kafka-go reader configured for the target topic/group.
	reader *kafka.Reader
	// logRepo persists request_log rows.
	logRepo *repository.RequestLogRepository
	// costRepo persists cost_record rows.
	costRepo *repository.CostRecordRepository
	// broadcaster fans out committed TransactionEvents to SSE clients.
	// May be nil when no SSE broadcaster is configured.
	broadcaster *sse.Broadcaster
	// log is the structured logger for consumer events.
	log zerolog.Logger
}

// NewConsumer constructs a Consumer connected to the given Kafka brokers.
//
// brokers is a comma-separated list of host:port pairs, e.g. "192.168.32.161:9094".
// topic is the Kafka topic to consume, e.g. "llm.transactions".
// groupID is the consumer group ID; pass "" to use the default "central-server-consumer".
// logRepo and costRepo must be non-nil.
// broadcaster may be nil — when set, committed events are broadcast to SSE clients.
func NewConsumer(
	brokers, topic, groupID string,
	logRepo *repository.RequestLogRepository,
	costRepo *repository.CostRecordRepository,
	broadcaster *sse.Broadcaster,
	log zerolog.Logger,
) *Consumer {
	if groupID == "" {
		groupID = defaultGroupID
	}

	// parse comma-separated broker list.
	brokerList := parseBrokers(brokers)

	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        brokerList,
		Topic:          topic,
		GroupID:        groupID,
		MinBytes:       1,
		MaxBytes:       10 << 20, // 10 MiB
		CommitInterval: 0,        // manual commit only (commit after successful insert)
		StartOffset:    kafka.LastOffset,
	})

	return &Consumer{
		reader:      reader,
		logRepo:     logRepo,
		costRepo:    costRepo,
		broadcaster: broadcaster,
		log:         log.With().Str("component", "kafka.consumer").Str("topic", topic).Logger(),
	}
}

// Start begins consuming messages from Kafka and blocks until ctx is cancelled.
// Each message is deserialised as a TransactionEvent, then both request_log and
// cost_record rows are inserted before the offset is committed.
//
// If the context is cancelled (e.g. SIGTERM), Start drains the in-flight message
// and returns ctx.Err().
//
// Panics inside the processing loop are recovered and logged; the consumer
// continues to the next message rather than crashing.
func (c *Consumer) Start(ctx context.Context) error {
	c.log.Info().Msg("kafka consumer started")
	defer c.log.Info().Msg("kafka consumer stopped")
	defer func() {
		if err := c.reader.Close(); err != nil {
			c.log.Error().Err(err).Msg("error closing kafka reader")
		}
	}()

	for {
		// check for context cancellation before blocking on FetchMessage.
		select {
		case <-ctx.Done():
			return fmt.Errorf("Consumer.Start: %w", ctx.Err())
		default:
		}

		// use a short-lived context for each fetch so we can check ctx.Done()
		// periodically without blocking forever.
		fetchCtx, cancel := context.WithTimeout(ctx, readTimeout)
		msg, err := c.reader.FetchMessage(fetchCtx)
		cancel()

		if err != nil {
			if errors.Is(err, context.DeadlineExceeded) {
				// read timeout — loop and check ctx.Done().
				continue
			}
			if errors.Is(err, context.Canceled) {
				return fmt.Errorf("Consumer.Start: context cancelled: %w", err)
			}
			c.log.Error().Err(err).Msg("error fetching kafka message")
			continue
		}

		// process in a protected call so a single bad message cannot crash the consumer.
		c.safeProcess(ctx, msg)
	}
}

// safeProcess wraps processMessage with panic recovery so a single malformed
// event cannot terminate the consumer goroutine.
func (c *Consumer) safeProcess(ctx context.Context, msg kafka.Message) {
	defer func() {
		if r := recover(); r != nil {
			c.log.Error().
				Interface("panic", r).
				Int64("offset", msg.Offset).
				Msg("panic recovered in consumer; skipping message")
		}
	}()

	if err := c.processMessage(ctx, msg); err != nil {
		c.log.Error().
			Err(err).
			Int64("offset", msg.Offset).
			Msg("failed to process kafka message")
		// do NOT commit offset on failure — the message will be re-delivered.
		return
	}

	// commit offset AFTER successful DB inserts.
	if err := c.reader.CommitMessages(ctx, msg); err != nil {
		c.log.Error().
			Err(err).
			Int64("offset", msg.Offset).
			Msg("failed to commit kafka offset")
	}
}

// processMessage deserialises a Kafka message as a TransactionEvent and inserts
// both a request_log row and a cost_record row into PostgreSQL.
// Both inserts use ON CONFLICT DO NOTHING for idempotency.
func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message) error {
	var event model.TransactionEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("processMessage: unmarshal: %w", err)
	}

	if err := c.logRepo.Insert(ctx, &event); err != nil {
		return fmt.Errorf("processMessage: insert request_log: %w", err)
	}

	if err := c.costRepo.Insert(ctx, &event); err != nil {
		return fmt.Errorf("processMessage: insert cost_record: %w", err)
	}

	c.log.Debug().
		Str("event_id", event.EventID).
		Str("request_id", event.RequestID).
		Str("user_id", event.UserID).
		Msg("transaction event persisted")

	// fan-out to SSE clients after the DB commit succeeds.
	if c.broadcaster != nil {
		if data, jsonErr := json.Marshal(event); jsonErr == nil {
			c.broadcaster.Broadcast(sse.Event{Type: "transaction", Data: string(data)})
		}
	}

	return nil
}

// parseBrokers splits a comma-separated broker string into a slice of addresses.
func parseBrokers(brokers string) []string {
	var result []string
	start := 0
	for i := 0; i < len(brokers); i++ {
		if brokers[i] == ',' {
			if b := trim(brokers[start:i]); b != "" {
				result = append(result, b)
			}
			start = i + 1
		}
	}
	if b := trim(brokers[start:]); b != "" {
		result = append(result, b)
	}
	return result
}

// trim removes leading and trailing ASCII space characters.
func trim(s string) string {
	for len(s) > 0 && s[0] == ' ' {
		s = s[1:]
	}
	for len(s) > 0 && s[len(s)-1] == ' ' {
		s = s[:len(s)-1]
	}
	return s
}
