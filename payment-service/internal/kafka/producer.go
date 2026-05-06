package kafka

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"time"

	"github.com/segmentio/kafka-go"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/kafka/event"
)

const (
	topicCompleted = "payments.completed"
	topicFailed    = "payments.failed"
	topicDLQ       = "payments.dlq"
)

// Producer holds one writer per outbound topic. All writes are synchronous
// (Async: false) so workers know immediately if publish failed.
type Producer struct {
	completed *kafka.Writer
	failed    *kafka.Writer
	dlq       *kafka.Writer
}

func newWriter(brokers []string, topic string) *kafka.Writer {
	return &kafka.Writer{
		Addr:         kafka.TCP(brokers...),
		Topic:        topic,
		RequiredAcks: kafka.RequireAll,
		BatchTimeout: 10 * time.Millisecond,
		Async:        false,
	}
}

// NewProducer creates writers for payments.completed, payments.failed, payments.dlq.
func NewProducer(brokersCSV string) *Producer {
	brokers := strings.Split(brokersCSV, ",")
	return &Producer{
		completed: newWriter(brokers, topicCompleted),
		failed:    newWriter(brokers, topicFailed),
		dlq:       newWriter(brokers, topicDLQ),
	}
}

// PublishCompleted sends a PaymentCompletedEvent keyed by orderId.
func (p *Producer) PublishCompleted(ctx context.Context, evt event.PaymentCompletedEvent, correlationID string) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := kafka.Message{
		Key:   []byte(evt.OrderID.String()),
		Value: body,
		Headers: []kafka.Header{
			{Key: "X-Correlation-ID", Value: []byte(correlationID)},
			// Spring JsonDeserializer requires __TypeId__ to resolve the target class.
			{Key: "__TypeId__", Value: []byte("com.ecommerce.order_service.kafka.event.PaymentCompletedEvent")},
		},
	}
	if err := p.completed.WriteMessages(ctx, msg); err != nil {
		slog.Error("kafka.producer: publish completed failed",
			"correlationId", correlationID, "orderId", evt.OrderID, "error", err)
		return err
	}
	slog.Info("kafka.producer: published payments.completed",
		"correlationId", correlationID, "orderId", evt.OrderID)
	return nil
}

// PublishFailed sends a PaymentFailedEvent keyed by orderId.
func (p *Producer) PublishFailed(ctx context.Context, evt event.PaymentFailedEvent, correlationID string) error {
	body, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	msg := kafka.Message{
		Key:   []byte(evt.OrderID.String()),
		Value: body,
		Headers: []kafka.Header{
			{Key: "X-Correlation-ID", Value: []byte(correlationID)},
			{Key: "__TypeId__", Value: []byte("com.ecommerce.order_service.kafka.event.PaymentFailedEvent")},
		},
	}
	if err := p.failed.WriteMessages(ctx, msg); err != nil {
		slog.Error("kafka.producer: publish failed-event failed",
			"correlationId", correlationID, "orderId", evt.OrderID, "error", err)
		return err
	}
	slog.Info("kafka.producer: published payments.failed",
		"correlationId", correlationID, "orderId", evt.OrderID)
	return nil
}

// PublishDLQ sends raw bytes (original message payload) to the dead-letter queue.
// Used by Week 11 retry/DLQ logic; wired now so the writer is ready.
func (p *Producer) PublishDLQ(ctx context.Context, raw []byte, reason, correlationID string) error {
	msg := kafka.Message{
		Value: raw,
		Headers: []kafka.Header{
			{Key: "X-Correlation-ID", Value: []byte(correlationID)},
			{Key: "X-DLQ-Reason", Value: []byte(reason)},
		},
	}
	if err := p.dlq.WriteMessages(ctx, msg); err != nil {
		slog.Error("kafka.producer: publish DLQ failed",
			"correlationId", correlationID, "reason", reason, "error", err)
		return err
	}
	slog.Warn("kafka.producer: message sent to DLQ",
		"correlationId", correlationID, "reason", reason)
	return nil
}

// Close flushes and closes all writers. Call after consumer workers have exited.
func (p *Producer) Close() {
	p.completed.Close()
	p.failed.Close()
	p.dlq.Close()
}
