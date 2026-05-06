package kafka

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/segmentio/kafka-go"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/config"
	kafkaevent "github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/kafka/event"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/service"
)

type contextKey string

const correlationIDKey contextKey = "correlationId"

// Consumer subscribes to orders.created and dispatches to a worker pool.
type Consumer struct {
	reader   *kafka.Reader
	producer *Producer
	svc      service.PaymentService
	cfg      *config.Config
	jobs     chan kafka.Message
	wg       sync.WaitGroup
}

// NewConsumer creates a Consumer with a manual-commit reader and a buffered job channel.
func NewConsumer(cfg *config.Config, svc service.PaymentService, prod *Producer) *Consumer {
	reader := kafka.NewReader(kafka.ReaderConfig{
		Brokers:        []string{cfg.KafkaBrokers},
		Topic:          "orders.created",
		GroupID:        cfg.KafkaConsumerGroup,
		MinBytes:       1,
		MaxBytes:       10 << 20,               // 10 MB
		MaxWait:        500 * time.Millisecond, // return promptly when messages arrive
		CommitInterval: 0,                      // manual commit only
		StartOffset:    kafka.FirstOffset,      // auto.offset.reset=earliest: process unacked messages after restart
	})
	return &Consumer{
		reader:   reader,
		producer: prod,
		svc:      svc,
		cfg:      cfg,
		jobs:     make(chan kafka.Message, 100),
	}
}

// Run starts the fetch loop and worker pool. Blocks until ctx is cancelled and all
// workers have drained. Call in a separate goroutine.
func (c *Consumer) Run(ctx context.Context) {
	slog.Info("kafka.consumer: started",
		"group", c.cfg.KafkaConsumerGroup,
		"topic", "orders.created",
		"workers", c.cfg.KafkaWorkerCount)

	for i := 0; i < c.cfg.KafkaWorkerCount; i++ {
		c.wg.Add(1)
		go c.runWorker(ctx, i)
	}

	// Fetch loop: push messages onto the jobs channel until ctx is cancelled.
	for {
		msg, err := c.reader.FetchMessage(ctx)
		if err != nil {
			// ctx cancelled or reader closed — normal shutdown path
			break
		}
		c.jobs <- msg
	}

	// Signal workers that no more messages are coming.
	close(c.jobs)
	c.wg.Wait()
	c.reader.Close()
	slog.Info("kafka.consumer: stopped")
}

// Wait blocks until all workers have exited. Use in the shutdown sequence after
// cancelling the consumer context.
func (c *Consumer) Wait() {
	c.wg.Wait()
}

func (c *Consumer) runWorker(ctx context.Context, workerID int) {
	defer c.wg.Done()

	for msg := range c.jobs {
		c.processMessage(ctx, msg, workerID)
	}
}

func (c *Consumer) processMessage(ctx context.Context, msg kafka.Message, workerID int) {
	correlationID := readHeader(msg, "X-Correlation-ID")
	if correlationID == "" {
		correlationID = uuid.NewString()
	}
	msgCtx := context.WithValue(ctx, correlationIDKey, correlationID)

	logBase := []any{
		"correlationId", correlationID,
		"workerId", workerID,
		"topic", msg.Topic,
		"partition", msg.Partition,
		"offset", msg.Offset,
	}

	var evt kafkaevent.OrderCreatedEvent
	if err := json.Unmarshal(msg.Value, &evt); err != nil {
		// Poison pill — log and commit. Week 11 will route to DLQ instead.
		slog.Error("kafka.worker: deserialize failed — committing poison pill",
			append(logBase, "error", err)...)
		_ = c.reader.CommitMessages(msgCtx, msg)
		return
	}

	logBase = append(logBase, "orderId", evt.OrderID)

	payment, err := c.svc.ProcessPayment(msgCtx, service.ProcessPaymentInput{
		OrderID:        evt.OrderID,
		UserID:         evt.UserID,
		Amount:         evt.TotalAmount,
		Currency:       "USD",
		IdempotencyKey: evt.OrderID.String(), // ADR locking-strategy §5
	})
	if err != nil {
		// Transient error — log and commit. Week 11 will add retry + DLQ.
		slog.Error("kafka.worker: ProcessPayment failed — committing for now",
			append(logBase, "error", err)...)
		_ = c.reader.CommitMessages(msgCtx, msg)
		return
	}

	// Publish the outcome event so order-service can transition order status.
	if publishErr := c.publishOutcome(msgCtx, payment, correlationID); publishErr != nil {
		slog.Error("kafka.worker: publish outcome failed",
			append(logBase, "paymentStatus", payment.Status, "error", publishErr)...)
		// Still commit — the payment row is persisted; a duplicate Kafka delivery
		// would hit the idempotency key and return the same outcome.
	}

	if commitErr := c.reader.CommitMessages(msgCtx, msg); commitErr != nil {
		slog.Error("kafka.worker: commit failed", append(logBase, "error", commitErr)...)
		return
	}

	slog.Info("kafka.worker: processed",
		append(logBase, "paymentStatus", payment.Status)...)
}

func (c *Consumer) publishOutcome(ctx context.Context, payment *model.Payment, correlationID string) error {
	switch payment.Status {
	case model.PaymentStatusCompleted:
		return c.producer.PublishCompleted(ctx, kafkaevent.PaymentCompletedEvent{
			OrderID:   payment.OrderID,
			PaymentID: payment.ID,
			Amount:    payment.Amount,
		}, correlationID)
	case model.PaymentStatusFailed:
		return c.producer.PublishFailed(ctx, kafkaevent.PaymentFailedEvent{
			OrderID: payment.OrderID,
			Reason:  fmt.Sprintf("gateway declined (paymentId=%s)", payment.ID),
		}, correlationID)
	default:
		// PENDING means ProcessPayment left it in a transient state — not a publish error.
		return nil
	}
}

func readHeader(msg kafka.Message, key string) string {
	for _, h := range msg.Headers {
		if h.Key == key {
			return string(h.Value)
		}
	}
	return ""
}
