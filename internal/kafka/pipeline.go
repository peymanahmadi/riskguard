package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"

	"github.com/segmentio/kafka-go"

	"github.com/peymanahmadi/riskguard/pkg/riskguard"
)

// Pipeline reads TransactionEvents from an input topic, evaluates each one
// through a riskguard.Engine, persists the transaction, and publishes the
// resulting DecisionEvent to an output topic.
type Pipeline struct {
	reader  *kafka.Reader
	writer  *kafka.Writer
	engine  *riskguard.Engine
	history riskguard.HistoryStore
	logger  *slog.Logger
}

// Config configures a Pipeline's Kafka connectivity.
type Config struct {
	Brokers       []string
	InputTopic    string // transactions
	OutputTopic   string // decisions
	ConsumerGroup string
}

func NewPipeline(cfg Config, engine *riskguard.Engine, history riskguard.HistoryStore, logger *slog.Logger) *Pipeline {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers: cfg.Brokers,
		Topic:   cfg.InputTopic,
		GroupID: cfg.ConsumerGroup,
		// MinBytes/MaxBytes tuned for low-latency small JSON payloads rather
		// than batch throughput; adjust for your traffic profile.
		MinBytes: 1,
		MaxBytes: 1 << 20,
	})

	writer := &kafka.Writer{
		Addr:         kafka.TCP(cfg.Brokers...),
		Topic:        cfg.OutputTopic,
		Balancer:     &kafka.Hash{}, // partition by key (entity id) for ordering per entity
		RequiredAcks: kafka.RequireOne,
	}

	if logger == nil {
		logger = slog.Default()
	}

	return &Pipeline{reader: reader, writer: writer, engine: engine, history: history, logger: logger}
}

// Run blocks, consuming and processing messages until ctx is cancelled or an
// unrecoverable read error occurs.
func (p *Pipeline) Run(ctx context.Context) error {
	defer func() {
		if err := p.reader.Close(); err != nil {
			p.logger.Error("failed to close kafka reader", "error", err)
		}
	}()
	defer func() {
		if err := p.writer.Close(); err != nil {
			p.logger.Error("failed to close kafka writer", "error", err)
		}
	}()

	for {
		msg, err := p.reader.FetchMessage(ctx)
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("kafka: fetch message: %w", err)
		}

		if err := p.handle(ctx, msg); err != nil {
			// Log and continue: one malformed/failed message should not
			// stall the whole consumer group. A production system would
			// route this to a dead-letter topic instead of just logging.
			p.logger.Error("failed to process transaction event",
				"error", err, "partition", msg.Partition, "offset", msg.Offset)
			continue
		}

		if err := p.reader.CommitMessages(ctx, msg); err != nil {
			p.logger.Error("failed to commit offset", "error", err)
		}
	}
}

func (p *Pipeline) handle(ctx context.Context, msg kafka.Message) error {
	var event TransactionEvent
	if err := json.Unmarshal(msg.Value, &event); err != nil {
		return fmt.Errorf("unmarshal transaction event: %w", err)
	}

	tx := event.toTransaction()

	verdict, err := p.engine.Evaluate(ctx, tx)
	if err != nil {
		// Engine still returns a usable verdict under FailOpen even when an
		// error is returned, so we log the degraded coverage but keep going.
		p.logger.Warn("engine evaluation reported rule errors", "tx_id", tx.ID, "error", err)
	}

	if p.history != nil {
		if saveErr := p.history.SaveTransaction(ctx, tx); saveErr != nil {
			p.logger.Error("failed to persist transaction", "tx_id", tx.ID, "error", saveErr)
		}
	}

	decision := newDecisionEvent(tx, verdict)
	payload, err := json.Marshal(decision)
	if err != nil {
		return fmt.Errorf("marshal decision event: %w", err)
	}

	return p.writer.WriteMessages(ctx, kafka.Message{
		Key:   []byte(tx.EntityID),
		Value: payload,
	})
}

// PublishTransaction is a small helper for producers (e.g. the demo HTTP
// server, or a load-test script) to publish a TransactionEvent onto the
// input topic instead of calling the engine synchronously.
func PublishTransaction(ctx context.Context, brokers []string, topic string, event TransactionEvent) (err error) {
	w := &kafka.Writer{Addr: kafka.TCP(brokers...), Topic: topic, Balancer: &kafka.Hash{}}
	defer func() {
		// kafka-go's Writer.Close flushes any buffered messages, so a close
		// error here can mean a message was never actually sent even though
		// WriteMessages returned nil. Surface it rather than discard it.
		if closeErr := w.Close(); closeErr != nil && err == nil {
			err = fmt.Errorf("close writer: %w", closeErr)
		}
	}()

	payload, marshalErr := json.Marshal(event)
	if marshalErr != nil {
		return fmt.Errorf("marshal transaction event: %w", marshalErr)
	}
	return w.WriteMessages(ctx, kafka.Message{Key: []byte(event.EntityID), Value: payload})
}
