package kafka

import (
	"context"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/larek-tech/storage/kafka"

// KeyExtractor извлекает partition key из значения.
type KeyExtractor[T any] func(T) []byte

// Producer[T] — типизированный продьюсер для одного топика.
type Producer[T any] struct {
	tr      Transport
	topic   string
	codec   Codec[T]
	keyFn   KeyExtractor[T]
	tracer  trace.Tracer
	withTel bool
}

type ProducerOption[T any] func(*Producer[T])

func WithProducerCodec[T any](codec Codec[T]) ProducerOption[T] {
	return func(p *Producer[T]) { p.codec = codec }
}

func WithKeyExtractor[T any](fn KeyExtractor[T]) ProducerOption[T] {
	return func(p *Producer[T]) { p.keyFn = fn }
}

func WithProducerTelemetry[T any](enabled bool) ProducerOption[T] {
	return func(p *Producer[T]) { p.withTel = enabled }
}

func WithProducerTracer[T any](tracer trace.Tracer) ProducerOption[T] {
	return func(p *Producer[T]) { p.tracer = tracer }
}

// NewProducer создаёт типизированный продьюсер. По умолчанию используется JSONCodec[T].
func NewProducer[T any](tr Transport, topic string, opts ...ProducerOption[T]) *Producer[T] {
	p := &Producer[T]{
		tr:     tr,
		topic:  topic,
		codec:  JSONCodec[T]{},
		tracer: otel.Tracer(tracerName),
	}
	for _, opt := range opts {
		opt(p)
	}
	return p
}

// Produce кодирует и отправляет батч сообщений в топик.
func (p *Producer[T]) Produce(ctx context.Context, msgs []T) error {
	ctx, span := p.startSpan(ctx, "Kafka.Produce",
		attribute.String("topic", p.topic),
		attribute.Int("batch_size", len(msgs)))
	defer span.End()

	if len(msgs) == 0 {
		return nil
	}

	raws := make([]RawMessage, len(msgs))
	for i, m := range msgs {
		payload, err := p.codec.Marshal(m)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
			return err
		}
		raws[i] = RawMessage{Topic: p.topic, Value: payload}
		if p.keyFn != nil {
			raws[i].Key = p.keyFn(m)
		}
	}

	if err := p.tr.Produce(ctx, p.topic, raws); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

func (p *Producer[T]) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !p.withTel {
		return ctx, trace.SpanFromContext(ctx)
	}
	return p.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}
