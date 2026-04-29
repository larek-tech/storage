package redis

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
)

// SetJSON сериализует value через encoding/json и сохраняет под ключом key.
// expiration <= 0 означает отсутствие TTL.
func (c *Client) SetJSON(ctx context.Context, key string, value any, expiration time.Duration) error {
	ctx, span := c.startSpan(ctx, "Redis.SetJSON",
		attribute.String("key", key),
		attribute.Int64("ttl_ms", expiration.Milliseconds()))
	defer span.End()

	payload, err := json.Marshal(value)
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("marshal json: %w", err)
	}

	if err := c.rdb.Set(ctx, key, payload, expiration).Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// GetJSON читает значение по ключу и десериализует его в dst через encoding/json.
// Если ключ отсутствует, возвращает redis.Nil без записи ошибки в span.
// dst должен быть указателем на принимающее значение.
func (c *Client) GetJSON(ctx context.Context, key string, dst any) error {
	ctx, span := c.startSpan(ctx, "Redis.GetJSON", attribute.String("key", key))
	defer span.End()

	raw, err := c.rdb.Get(ctx, key).Bytes()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return err
	}

	if err := json.Unmarshal(raw, dst); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return fmt.Errorf("unmarshal json: %w", err)
	}
	return nil
}
