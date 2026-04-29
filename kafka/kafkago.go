package kafka

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"sync"
	"time"

	kgo "github.com/segmentio/kafka-go"
	"github.com/segmentio/kafka-go/sasl"
	"github.com/segmentio/kafka-go/sasl/plain"
	"github.com/segmentio/kafka-go/sasl/scram"
)

// KafkaGoTransport реализует Transport через github.com/segmentio/kafka-go.
type KafkaGoTransport struct {
	cfg     Cfg
	dialer  *kgo.Dialer
	mu      sync.Mutex
	writers map[string]*kgo.Writer
	closed  bool
}

// NewKafkaGoTransport создаёт транспорт и проверяет доступность брокеров
// через диалер (без явного ping — kafka-go не имеет такой команды).
func NewKafkaGoTransport(_ context.Context, cfg Cfg) (*KafkaGoTransport, error) {
	if len(cfg.Brokers) == 0 {
		return nil, errors.New("kafka: at least one broker required")
	}

	mech, err := saslMechanism(cfg)
	if err != nil {
		return nil, err
	}

	dialer := &kgo.Dialer{
		Timeout:       10 * time.Second,
		DualStack:     true,
		ClientID:      cfg.ClientID,
		SASLMechanism: mech,
	}
	if cfg.UseTLS {
		dialer.TLS = &tls.Config{MinVersion: tls.VersionTLS12}
	}

	return &KafkaGoTransport{
		cfg:     cfg,
		dialer:  dialer,
		writers: map[string]*kgo.Writer{},
	}, nil
}

func saslMechanism(cfg Cfg) (sasl.Mechanism, error) {
	switch cfg.SASL {
	case "":
		return nil, nil
	case "plain":
		return plain.Mechanism{Username: cfg.User, Password: cfg.Password}, nil
	case "scram-sha-256":
		return scram.Mechanism(scram.SHA256, cfg.User, cfg.Password)
	case "scram-sha-512":
		return scram.Mechanism(scram.SHA512, cfg.User, cfg.Password)
	default:
		return nil, fmt.Errorf("unsupported sasl mechanism %q", cfg.SASL)
	}
}

func (t *KafkaGoTransport) writerFor(topic string) *kgo.Writer {
	t.mu.Lock()
	defer t.mu.Unlock()
	if w, ok := t.writers[topic]; ok {
		return w
	}
	transport := &kgo.Transport{
		Dial:     t.dialer.DialFunc,
		ClientID: t.cfg.ClientID,
		SASL:     t.dialer.SASLMechanism,
		TLS:      t.dialer.TLS,
	}
	w := &kgo.Writer{
		Addr:         kgo.TCP(t.cfg.Brokers...),
		Topic:        topic,
		Balancer:     &kgo.Hash{},
		RequiredAcks: kgo.RequireAll,
		Transport:    transport,
	}
	t.writers[topic] = w
	return w
}

func (t *KafkaGoTransport) Produce(ctx context.Context, topic string, msgs []RawMessage) error {
	if len(msgs) == 0 {
		return nil
	}
	out := make([]kgo.Message, len(msgs))
	for i, m := range msgs {
		out[i] = kgo.Message{
			Key:     m.Key,
			Value:   m.Value,
			Headers: toKgoHeaders(m.Headers),
			Time:    m.Timestamp,
		}
	}
	return t.writerFor(topic).WriteMessages(ctx, out...)
}

// Consume читает сообщения из топика consumer-group'ой cfg.GroupID и
// вызывает handler по одному сообщению за раз. Оффсет коммитится только
// если handler вернул processed >= 1 без ошибки.
func (t *KafkaGoTransport) Consume(ctx context.Context, topic string, handler func(msgs []RawMessage) (int, error)) error {
	if t.cfg.GroupID == "" {
		return errors.New("kafka: GroupID is required to consume")
	}
	r := kgo.NewReader(kgo.ReaderConfig{
		Brokers: t.cfg.Brokers,
		GroupID: t.cfg.GroupID,
		Topic:   topic,
		Dialer:  t.dialer,
	})
	defer func() { _ = r.Close() }()

	for {
		m, err := r.FetchMessage(ctx)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil
			}
			return err
		}
		raw := RawMessage{
			Key:       m.Key,
			Value:     m.Value,
			Headers:   fromKgoHeaders(m.Headers),
			Topic:     m.Topic,
			Partition: safeInt32(m.Partition),
			Offset:    m.Offset,
			Timestamp: m.Time,
		}
		processed, hErr := handler([]RawMessage{raw})
		if hErr == nil && processed >= 1 {
			if err := r.CommitMessages(ctx, m); err != nil {
				return err
			}
		}
	}
}

func (t *KafkaGoTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.closed {
		return nil
	}
	t.closed = true
	var errs error
	for _, w := range t.writers {
		if err := w.Close(); err != nil {
			errs = errors.Join(errs, err)
		}
	}
	return errs
}

func toKgoHeaders(h map[string]string) []kgo.Header {
	if len(h) == 0 {
		return nil
	}
	out := make([]kgo.Header, 0, len(h))
	for k, v := range h {
		out = append(out, kgo.Header{Key: k, Value: []byte(v)})
	}
	return out
}

func safeInt32(v int) int32 {
	const maxI32 = int(^uint32(0) >> 1)
	const minI32 = -maxI32 - 1
	if v > maxI32 {
		return int32(maxI32)
	}
	if v < minI32 {
		return int32(minI32)
	}
	return int32(v)
}

func fromKgoHeaders(h []kgo.Header) map[string]string {
	if len(h) == 0 {
		return nil
	}
	out := make(map[string]string, len(h))
	for _, kv := range h {
		out[kv.Key] = string(kv.Value)
	}
	return out
}
