package kafka

import (
	"context"
	"time"
)

// RawMessage — низкоуровневое сообщение шины (байты + метаданные).
type RawMessage struct {
	Key       []byte
	Value     []byte
	Headers   map[string]string
	Topic     string
	Partition int32
	Offset    int64
	Timestamp time.Time
}

// Transport абстрагирует транспорт шины (Kafka/Valkey-stream/in-memory).
// Дженерики живут только в верхнем слое; транспорт работает с байтами.
type Transport interface {
	Produce(ctx context.Context, topic string, msgs []RawMessage) error
	Consume(ctx context.Context, topic string, handler func(msgs []RawMessage) (int, error)) error
	Close() error
}
