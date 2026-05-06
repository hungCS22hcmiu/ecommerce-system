//go:build integration

package integration_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/shopspring/decimal"
	segkafka "github.com/segmentio/kafka-go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	tcKafka "github.com/testcontainers/testcontainers-go/modules/kafka"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	gormpg "gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"

	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/config"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/dto"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/gateway"
	kafkaevent "github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/kafka/event"
	kafkapkg "github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/kafka"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/model"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/repository"
	"github.com/hungCS22hcmiu/ecommrece-system/payment-service/internal/service"
)

// ─── Test helpers ──────────────────────────────────────────────────────────────

// startKafkaBroker starts a Kafka testcontainer and returns the broker address.
func startKafkaBroker(t *testing.T) string {
	t.Helper()
	ctx := context.Background()

	container, err := tcKafka.RunContainer(ctx)
	require.NoError(t, err, "start Kafka container")
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	brokers, err := container.Brokers(ctx)
	require.NoError(t, err, "get Kafka brokers")
	require.NotEmpty(t, brokers)
	return brokers[0]
}

// ensureTopics pre-creates topics via the Kafka controller connection so tests
// don't race against auto-creation.
func ensureTopics(t *testing.T, brokerAddr string, topics ...string) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	conn, err := segkafka.DialContext(ctx, "tcp", brokerAddr)
	require.NoError(t, err, "dial Kafka for topic creation")
	defer conn.Close()

	ctrl, err := conn.Controller()
	require.NoError(t, err, "get Kafka controller")
	ctrlConn, err := segkafka.DialContext(ctx, "tcp", fmt.Sprintf("%s:%d", ctrl.Host, ctrl.Port))
	require.NoError(t, err, "dial Kafka controller")
	defer ctrlConn.Close()

	specs := make([]segkafka.TopicConfig, len(topics))
	for i, topic := range topics {
		specs[i] = segkafka.TopicConfig{Topic: topic, NumPartitions: 1, ReplicationFactor: 1}
	}
	_ = ctrlConn.CreateTopics(specs...) // tolerate "already exists"
}

// testCfg returns a Config pointing at the given broker with a unique consumer group.
func testCfg(brokerAddr string) *config.Config {
	return &config.Config{
		KafkaBrokers:       brokerAddr,
		KafkaConsumerGroup: "test-" + uuid.NewString(),
		KafkaWorkerCount:   2,
	}
}

// publishMsg writes a single message to the given topic on the broker.
func publishMsg(t *testing.T, brokerAddr, topic string, payload []byte) {
	t.Helper()
	w := &segkafka.Writer{
		Addr:         segkafka.TCP(brokerAddr),
		Topic:        topic,
		RequiredAcks: segkafka.RequireOne,
		BatchTimeout: 10 * time.Millisecond,
	}
	t.Cleanup(func() { _ = w.Close() })

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	require.NoError(t, w.WriteMessages(ctx, segkafka.Message{Value: payload}), "publish test message")
}

// consumeOne reads the first message from a topic, returning nil if none arrives
// within timeout. Uses a fresh group per call to always read from offset 0.
func consumeOne(t *testing.T, brokerAddr, topic string, timeout time.Duration) []byte {
	t.Helper()
	r := segkafka.NewReader(segkafka.ReaderConfig{
		Brokers:     []string{brokerAddr},
		Topic:       topic,
		GroupID:     "assert-" + uuid.NewString(),
		StartOffset: segkafka.FirstOffset,
		MaxWait:     500 * time.Millisecond,
		MinBytes:    1,
		MaxBytes:    10 << 20,
	})
	defer r.Close()

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	msg, err := r.ReadMessage(ctx)
	if err != nil {
		return nil
	}
	return msg.Value
}

// startConsumer wires a Consumer and starts it in a background goroutine.
// The returned cancel func stops the consumer; t.Cleanup calls it automatically.
func startConsumer(t *testing.T, cfg *config.Config, svc service.PaymentService) context.CancelFunc {
	t.Helper()
	producer := kafkapkg.NewProducer(cfg.KafkaBrokers)
	consumer := kafkapkg.NewConsumer(cfg, svc, producer)
	ctx, cancel := context.WithCancel(context.Background())
	go consumer.Run(ctx)
	t.Cleanup(func() {
		cancel()
		consumer.Wait()
		producer.Close()
	})
	return cancel
}

// ─── Mock service ─────────────────────────────────────────────────────────────

// stubSvc implements service.PaymentService with an injected ProcessPayment func.
type stubSvc struct {
	fn func(context.Context, service.ProcessPaymentInput) (*model.Payment, error)
}

func (s *stubSvc) ProcessPayment(ctx context.Context, in service.ProcessPaymentInput) (*model.Payment, error) {
	return s.fn(ctx, in)
}
func (s *stubSvc) GetByID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.PaymentResponse, error) {
	panic("not in scope for Kafka tests")
}
func (s *stubSvc) GetByOrderID(context.Context, uuid.UUID, uuid.UUID, bool) (*dto.PaymentResponse, error) {
	panic("not in scope for Kafka tests")
}
func (s *stubSvc) ListByUser(context.Context, uuid.UUID, int, int) ([]dto.PaymentResponse, int64, error) {
	panic("not in scope for Kafka tests")
}

// ─── Zero-latency gateway for test 4 ─────────────────────────────────────────

type instantGateway struct{}

func (g *instantGateway) Charge(_ context.Context, _ decimal.Decimal, _, _ string) (string, error) {
	return "MOCK-" + uuid.NewString(), nil
}

var _ gateway.Gateway = (*instantGateway)(nil)

// ─── Tests ────────────────────────────────────────────────────────────────────

// TestDLQOnPoisonPill: a malformed JSON payload routes straight to payments.dlq
// without calling ProcessPayment, and the consumer stays alive.
func TestDLQOnPoisonPill(t *testing.T) {
	brokerAddr := startKafkaBroker(t)
	ensureTopics(t, brokerAddr, "orders.created", "payments.completed", "payments.failed", "payments.dlq")

	called := false
	svc := &stubSvc{fn: func(ctx context.Context, in service.ProcessPaymentInput) (*model.Payment, error) {
		called = true
		t.Error("ProcessPayment must not be called for a poison-pill message")
		return nil, nil
	}}
	startConsumer(t, testCfg(brokerAddr), svc)

	badPayload := []byte("NOT_JSON{{{")
	publishMsg(t, brokerAddr, "orders.created", badPayload)

	// DLQ must receive the message within 5s.
	raw := consumeOne(t, brokerAddr, "payments.dlq", 5*time.Second)
	require.NotNil(t, raw, "payments.dlq must receive the poison-pill message")

	var dlq struct {
		ErrorStage    string `json:"errorStage"`
		OriginalValue string `json:"originalValue"`
	}
	require.NoError(t, json.Unmarshal(raw, &dlq))
	assert.Equal(t, "deserialize", dlq.ErrorStage)
	assert.Equal(t, base64.StdEncoding.EncodeToString(badPayload), dlq.OriginalValue)

	// payments.completed must remain empty.
	assert.Nil(t, consumeOne(t, brokerAddr, "payments.completed", 500*time.Millisecond))
	assert.False(t, called, "ProcessPayment should not have been invoked")
}

// TestDLQAfterRetryExhaustion: a transient error on every attempt causes the
// consumer to exhaust its 3 retries and route to payments.dlq with errorStage="process".
func TestDLQAfterRetryExhaustion(t *testing.T) {
	brokerAddr := startKafkaBroker(t)
	ensureTopics(t, brokerAddr, "orders.created", "payments.completed", "payments.failed", "payments.dlq")

	svc := &stubSvc{fn: func(ctx context.Context, in service.ProcessPaymentInput) (*model.Payment, error) {
		return nil, errors.New("db: connection lost")
	}}
	startConsumer(t, testCfg(brokerAddr), svc)

	evt := kafkaevent.OrderCreatedEvent{
		OrderID:     uuid.New(),
		UserID:      uuid.New(),
		TotalAmount: decimal.NewFromFloat(50.00),
	}
	payload, _ := json.Marshal(evt)
	publishMsg(t, brokerAddr, "orders.created", payload)

	// Retry backoffs total ~700ms; give the consumer 6s.
	raw := consumeOne(t, brokerAddr, "payments.dlq", 6*time.Second)
	require.NotNil(t, raw, "payments.dlq must receive message after retry exhaustion")

	var dlq struct {
		ErrorStage string `json:"errorStage"`
		Attempts   int    `json:"attempts"`
	}
	require.NoError(t, json.Unmarshal(raw, &dlq))
	assert.Equal(t, "process", dlq.ErrorStage)
	assert.GreaterOrEqual(t, dlq.Attempts, 4, "expected at least 4 ProcessPayment calls before DLQ")

	// payments.completed must remain empty.
	assert.Nil(t, consumeOne(t, brokerAddr, "payments.completed", 500*time.Millisecond))
}

// TestPermanentDeclineNoDLQ: when ProcessPayment returns (FAILED payment, nil error)
// — mirroring the real service when the gateway declines — the consumer publishes
// to payments.failed and nothing goes to payments.dlq.
func TestPermanentDeclineNoDLQ(t *testing.T) {
	brokerAddr := startKafkaBroker(t)
	ensureTopics(t, brokerAddr, "orders.created", "payments.completed", "payments.failed", "payments.dlq")

	orderID := uuid.New()
	svc := &stubSvc{fn: func(ctx context.Context, in service.ProcessPaymentInput) (*model.Payment, error) {
		return &model.Payment{
			ID:      uuid.New(),
			OrderID: in.OrderID,
			UserID:  in.UserID,
			Amount:  in.Amount,
			Status:  model.PaymentStatusFailed,
		}, nil
	}}
	startConsumer(t, testCfg(brokerAddr), svc)

	evt := kafkaevent.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      uuid.New(),
		TotalAmount: decimal.NewFromFloat(75.00),
	}
	payload, _ := json.Marshal(evt)
	publishMsg(t, brokerAddr, "orders.created", payload)

	// payments.failed must receive a message.
	raw := consumeOne(t, brokerAddr, "payments.failed", 5*time.Second)
	require.NotNil(t, raw, "payments.failed must receive the decline event")

	var failed struct {
		OrderID string `json:"orderId"`
	}
	require.NoError(t, json.Unmarshal(raw, &failed))
	assert.Equal(t, orderID.String(), failed.OrderID)

	// payments.dlq must remain empty.
	assert.Nil(t, consumeOne(t, brokerAddr, "payments.dlq", 500*time.Millisecond),
		"permanent decline must NOT produce a DLQ entry")
	assert.Nil(t, consumeOne(t, brokerAddr, "payments.completed", 500*time.Millisecond))
}

// TestDuplicateDeliveryIdempotency: publishing the same OrderCreatedEvent 3 times
// (simulating at-least-once redelivery) must result in exactly one payment row in
// Postgres — the idempotency key prevents double-charging.
func TestDuplicateDeliveryIdempotency(t *testing.T) {
	brokerAddr := startKafkaBroker(t)
	ensureTopics(t, brokerAddr, "orders.created", "payments.completed", "payments.failed", "payments.dlq")
	ctx := context.Background()

	// Postgres container.
	pgCtr, err := tcpostgres.Run(ctx, "postgres:16-alpine",
		tcpostgres.WithDatabase("ecommerce_payments"),
		tcpostgres.WithUsername("postgres"),
		tcpostgres.WithPassword("postgres"),
	)
	require.NoError(t, err, "start Postgres container")
	t.Cleanup(func() { _ = pgCtr.Terminate(ctx) })

	dsn, err := pgCtr.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)

	db, err := gorm.Open(gormpg.Open(dsn), &gorm.Config{
		Logger: gormlogger.Default.LogMode(gormlogger.Silent),
	})
	require.NoError(t, err)
	sqlDB, _ := db.DB()
	sqlDB.SetMaxOpenConns(25)
	runMigrations(t, sqlDB, dsn)

	// Real service with a zero-latency always-succeed gateway.
	repo := repository.NewPaymentRepository(db)
	svc := service.NewPaymentService(repo, &instantGateway{})
	startConsumer(t, testCfg(brokerAddr), svc)

	// Publish the same event 3 times.
	orderID := uuid.New()
	evt := kafkaevent.OrderCreatedEvent{
		OrderID:     orderID,
		UserID:      uuid.New(),
		TotalAmount: decimal.NewFromFloat(99.99),
	}
	payload, _ := json.Marshal(evt)

	w := &segkafka.Writer{
		Addr:         segkafka.TCP(brokerAddr),
		Topic:        "orders.created",
		RequiredAcks: segkafka.RequireOne,
		BatchTimeout: 10 * time.Millisecond,
	}
	t.Cleanup(func() { _ = w.Close() })

	for i := 0; i < 3; i++ {
		require.NoError(t, w.WriteMessages(ctx, segkafka.Message{
			Key:   []byte(orderID.String()),
			Value: payload,
		}))
	}

	// Allow all 3 deliveries to be processed (zero gateway latency → fast).
	time.Sleep(3 * time.Second)

	// Exactly 1 payment row must exist.
	var paymentCount int64
	db.Model(&model.Payment{}).Where("order_id = ?", orderID).Count(&paymentCount)
	assert.Equal(t, int64(1), paymentCount, "idempotency: exactly one payment row for duplicate events")

	// The payment must be in a terminal state (not stuck in PENDING).
	var terminalCount int64
	db.Model(&model.Payment{}).
		Where("order_id = ? AND status IN ('COMPLETED','FAILED')", orderID).
		Count(&terminalCount)
	assert.Equal(t, int64(1), terminalCount, "payment must reach a terminal state")

	// No DLQ entries for this order.
	assert.Nil(t, consumeOne(t, brokerAddr, "payments.dlq", 500*time.Millisecond),
		"no DLQ entries expected on successful idempotent processing")
}
