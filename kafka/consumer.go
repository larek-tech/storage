package kafka

import (
	"context"
	"errors"
	"time"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

// Handler обрабатывает декодированный батч и возвращает число успешно
// обработанных сообщений (используется транспортом для коммита оффсетов,
// если он это поддерживает).
type Handler[T any] func(ctx context.Context, msgs []Message[T]) (int, error)

// Consumer[T] — типизированный консюмер одного топика.
type Consumer[T any] struct {
	tr      Transport
	topic   string
	codec   Codec[T]
	retry   RetryPolicy
	dlq     DLQ
	tracer  trace.Tracer
	withTel bool
}

type ConsumerOption[T any] func(*Consumer[T])

func WithConsumerCodec[T any](codec Codec[T]) ConsumerOption[T] {
	return func(c *Consumer[T]) { c.codec = codec }
}

func WithRetryPolicy[T any](rp RetryPolicy) ConsumerOption[T] {
	return func(c *Consumer[T]) { c.retry = rp }
}

func WithDLQ[T any](dlq DLQ) ConsumerOption[T] {
	return func(c *Consumer[T]) { c.dlq = dlq }
}

func WithConsumerTelemetry[T any](enabled bool) ConsumerOption[T] {
	return func(c *Consumer[T]) { c.withTel = enabled }
}

func WithConsumerTracer[T any](tracer trace.Tracer) ConsumerOption[T] {
	return func(c *Consumer[T]) { c.tracer = tracer }
}

func NewConsumer[T any](tr Transport, topic string, opts ...ConsumerOption[T]) *Consumer[T] {
	c := &Consumer[T]{
		tr:     tr,
		topic:  topic,
		codec:  JSONCodec[T]{},
		tracer: otel.Tracer(tracerName),
	}
	for _, opt := range opts {
		opt(c)
	}
	return c
}

// Consume запускает потребление сообщений до отмены ctx.
// На каждый батч вызывается handler; ошибки обрабатываются по RetryPolicy/DLQ:
//   - retry policy не задана → ошибка handler пробрасывается транспорту;
//   - retry policy задана: повтор до Next() == retry=false;
//   - после исчерпания ретраев и при наличии DLQ — все сообщения батча
//     отправляются в DLQ, ошибка не пробрасывается.
func (c *Consumer[T]) Consume(ctx context.Context, handler Handler[T]) error {
	return c.tr.Consume(ctx, c.topic, func(raws []RawMessage) (int, error) {
		ctx, span := c.startSpan(ctx, "Kafka.Consume",
			attribute.String("topic", c.topic),
			attribute.Int("batch_size", len(raws)))
		defer span.End()

		msgs := make([]Message[T], len(raws))
		for i, raw := range raws {
			msgs[i] = Message[T]{Raw: raw}
			if err := c.codec.Unmarshal(raw.Value, &msgs[i].Value); err != nil {
				msgs[i].UnmarshalErr = err
				span.RecordError(err)
			}
		}

		processed, err := c.runWithRetry(ctx, handler, msgs, span)
		if err != nil {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return processed, err
	})
}

func (c *Consumer[T]) runWithRetry(ctx context.Context, handler Handler[T], msgs []Message[T], span trace.Span) (int, error) {
	processed, err := handler(ctx, msgs)
	if err == nil {
		return processed, nil
	}

	if c.retry == nil {
		return processed, err
	}

	for attempt := 1; ; attempt++ {
		delay, retry := c.retry.Next(attempt, err)
		if !retry {
			break
		}
		span.AddEvent("retry", trace.WithAttributes(
			attribute.Int("attempt", attempt),
			attribute.Int64("delay_ms", delay.Milliseconds()),
		))
		if delay > 0 {
			t := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				t.Stop()
				return processed, ctx.Err()
			case <-t.C:
			}
		}
		processed, err = handler(ctx, msgs)
		if err == nil {
			return processed, nil
		}
	}

	if c.dlq != nil {
		var dlqErr error
		for _, m := range msgs {
			if sendErr := c.dlq.Send(m.Raw, err); sendErr != nil {
				dlqErr = errors.Join(dlqErr, sendErr)
			}
		}
		if dlqErr != nil {
			return processed, errors.Join(err, dlqErr)
		}
		return len(msgs), nil
	}

	return processed, err
}

func (c *Consumer[T]) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !c.withTel {
		return ctx, trace.SpanFromContext(ctx)
	}
	return c.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}
