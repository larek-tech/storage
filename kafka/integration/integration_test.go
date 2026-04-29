package integration_test

import (
	"context"
	"errors"
	"net"
	"strconv"
	"sync/atomic"
	"testing"
	"time"

	pkgkafka "github.com/larek-tech/storage/kafka"
	kgo "github.com/segmentio/kafka-go"
	"github.com/testcontainers/testcontainers-go"
	tckafka "github.com/testcontainers/testcontainers-go/modules/kafka"
)

type payload struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

func startKafka(t *testing.T) []string {
	t.Helper()
	ctx := context.Background()
	c, err := tckafka.Run(ctx, "confluentinc/confluent-local:8.0.0",
		tckafka.WithClusterID("test"),
	)
	if err != nil {
		t.Fatalf("start kafka: %v", err)
	}
	t.Cleanup(func() {
		_ = testcontainers.TerminateContainer(c)
	})
	brokers, err := c.Brokers(ctx)
	if err != nil {
		t.Fatalf("get brokers: %v", err)
	}
	return brokers
}

// createTopic создаёт топик через controller-соединение.
// Kafka 4.0 по умолчанию выключает auto.create.topics.enable.
func createTopic(t *testing.T, brokers []string, topic string) {
	t.Helper()
	conn, err := kgo.Dial("tcp", brokers[0])
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer func() { _ = conn.Close() }()

	controller, err := conn.Controller()
	if err != nil {
		t.Fatalf("controller: %v", err)
	}
	cConn, err := kgo.Dial("tcp", net.JoinHostPort(controller.Host, strconv.Itoa(controller.Port)))
	if err != nil {
		t.Fatalf("dial controller: %v", err)
	}
	defer func() { _ = cConn.Close() }()

	if err := cConn.CreateTopics(kgo.TopicConfig{
		Topic:             topic,
		NumPartitions:     1,
		ReplicationFactor: 1,
	}); err != nil {
		t.Fatalf("create topic: %v", err)
	}
}

func TestProduceConsumeRoundTrip(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	brokers := startKafka(t)
	const topic = "test-roundtrip"
	createTopic(t, brokers, topic)

	tr, err := pkgkafka.NewKafkaGoTransport(t.Context(), pkgkafka.Cfg{
		Brokers:  brokers,
		GroupID:  "test-group",
		ClientID: "test-client",
	})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	defer func() { _ = tr.Close() }()

	producer := pkgkafka.NewProducer[payload](tr, topic)
	if err := producer.Produce(t.Context(), []payload{
		{ID: 1, Name: "a"},
		{ID: 2, Name: "b"},
	}); err != nil {
		t.Fatalf("produce: %v", err)
	}

	consumer := pkgkafka.NewConsumer[payload](tr, topic)
	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	received := make(chan payload, 2)
	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, msgs []pkgkafka.Message[payload]) (int, error) {
			for _, m := range msgs {
				if m.UnmarshalErr != nil {
					t.Errorf("unmarshal: %v", m.UnmarshalErr)
					continue
				}
				received <- m.Value
			}
			return len(msgs), nil
		})
	}()

	got := map[int]string{}
	for range 2 {
		select {
		case v := <-received:
			got[v.ID] = v.Name
		case <-ctx.Done():
			t.Fatalf("timeout, got=%v", got)
		}
	}
	cancel()

	if got[1] != "a" || got[2] != "b" {
		t.Fatalf("unexpected payloads: %v", got)
	}
}

func TestRetryAndDLQ(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	brokers := startKafka(t)
	const topic = "test-dlq"
	createTopic(t, brokers, topic)

	tr, err := pkgkafka.NewKafkaGoTransport(t.Context(), pkgkafka.Cfg{
		Brokers: brokers, GroupID: "test-dlq-group", ClientID: "test-client",
	})
	if err != nil {
		t.Fatalf("transport: %v", err)
	}
	defer func() { _ = tr.Close() }()

	if err := pkgkafka.NewProducer[payload](tr, topic).
		Produce(t.Context(), []payload{{ID: 42, Name: "boom"}}); err != nil {
		t.Fatalf("produce: %v", err)
	}

	dlq := &pkgkafka.InMemoryDLQ{}
	consumer := pkgkafka.NewConsumer[payload](tr, topic,
		pkgkafka.WithRetryPolicy[payload](pkgkafka.InMemoryRetryPolicy{MaxAttempts: 3, BaseDelay: 10 * time.Millisecond}),
		pkgkafka.WithDLQ[payload](dlq),
	)

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
	defer cancel()

	var attempts atomic.Int32
	go func() {
		_ = consumer.Consume(ctx, func(_ context.Context, _ []pkgkafka.Message[payload]) (int, error) {
			attempts.Add(1)
			return 0, errors.New("always fail")
		})
	}()

	deadline := time.NewTimer(20 * time.Second)
	defer deadline.Stop()
	for len(dlq.Messages()) == 0 {
		select {
		case <-deadline.C:
			t.Fatalf("DLQ never received message; attempts=%d", attempts.Load())
		case <-time.After(100 * time.Millisecond):
		}
	}
	cancel()

	if got := attempts.Load(); got < 3 {
		t.Fatalf("expected at least 3 handler attempts, got %d", got)
	}
}
