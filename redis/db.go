package redis

import (
	"context"
	"crypto/tls"
	"errors"
	"time"

	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

const tracerName = "github.com/larek-tech/storage/redis"

// Config описывает источник DSN/настроек подключения.
type Config interface {
	DSN() string
}

// Client — обёртка над *redis.Client с опциональной OpenTelemetry-трассировкой.
type Client struct {
	rdb     *redis.Client
	tracer  trace.Tracer
	withTel bool
}

type Option func(*Client)

func WithTelemetry(enabled bool) Option {
	return func(c *Client) {
		c.withTel = enabled
	}
}

func WithTracer(tracer trace.Tracer) Option {
	return func(c *Client) {
		c.tracer = tracer
	}
}

// WithTLSConfig переопределяет TLS-конфигурацию подключения.
// По умолчанию TLS включается автоматически для схемы rediss://.
func WithTLSConfig(tlsCfg *tls.Config) Option {
	return func(c *Client) {
		c.rdb.Options().TLSConfig = tlsCfg
	}
}

// New создаёт клиента Redis/Valkey по DSN из cfg и проверяет соединение PING'ом.
func New(ctx context.Context, cfg Config, opts ...Option) (*Client, error) {
	rOpts, err := redis.ParseURL(cfg.DSN())
	if err != nil {
		return nil, err
	}

	rdb := redis.NewClient(rOpts)

	c := &Client{
		rdb:     rdb,
		tracer:  otel.Tracer(tracerName),
		withTel: false,
	}
	for _, opt := range opts {
		opt(c)
	}

	if err := rdb.Ping(ctx).Err(); err != nil {
		_ = rdb.Close()
		return nil, err
	}

	return c, nil
}

func (c *Client) startSpan(ctx context.Context, name string, attrs ...attribute.KeyValue) (context.Context, trace.Span) {
	if !c.withTel {
		return ctx, trace.SpanFromContext(ctx)
	}
	return c.tracer.Start(ctx, name, trace.WithAttributes(attrs...))
}

// GetClient возвращает нижележащий *redis.Client для расширенных операций.
func (c *Client) GetClient() *redis.Client {
	return c.rdb
}

func (c *Client) Close() error {
	return c.rdb.Close()
}

func (c *Client) Ping(ctx context.Context) error {
	ctx, span := c.startSpan(ctx, "Redis.Ping")
	defer span.End()

	if err := c.rdb.Ping(ctx).Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// Get возвращает строковое значение по ключу. Если ключ отсутствует,
// возвращает redis.Nil без записи ошибки в span.
func (c *Client) Get(ctx context.Context, key string) (string, error) {
	ctx, span := c.startSpan(ctx, "Redis.Get", attribute.String("key", key))
	defer span.End()

	val, err := c.rdb.Get(ctx, key).Result()
	if err != nil {
		if !errors.Is(err, redis.Nil) {
			span.RecordError(err)
			span.SetStatus(codes.Error, err.Error())
		}
		return "", err
	}
	return val, nil
}

// Set сохраняет значение по ключу. expiration <= 0 означает отсутствие TTL.
func (c *Client) Set(ctx context.Context, key string, value any, expiration time.Duration) error {
	ctx, span := c.startSpan(ctx, "Redis.Set",
		attribute.String("key", key),
		attribute.Int64("ttl_ms", expiration.Milliseconds()))
	defer span.End()

	if err := c.rdb.Set(ctx, key, value, expiration).Err(); err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return err
	}
	return nil
}

// Del удаляет указанные ключи и возвращает число фактически удалённых.
func (c *Client) Del(ctx context.Context, keys ...string) (int64, error) {
	ctx, span := c.startSpan(ctx, "Redis.Del", attribute.Int("keys_count", len(keys)))
	defer span.End()

	n, err := c.rdb.Del(ctx, keys...).Result()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	span.SetAttributes(attribute.Int64("deleted", n))
	return n, nil
}

// Exists возвращает число существующих ключей из переданных.
func (c *Client) Exists(ctx context.Context, keys ...string) (int64, error) {
	ctx, span := c.startSpan(ctx, "Redis.Exists", attribute.Int("keys_count", len(keys)))
	defer span.End()

	n, err := c.rdb.Exists(ctx, keys...).Result()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return 0, err
	}
	return n, nil
}

// Expire устанавливает TTL для ключа. Возвращает true, если TTL применён.
func (c *Client) Expire(ctx context.Context, key string, expiration time.Duration) (bool, error) {
	ctx, span := c.startSpan(ctx, "Redis.Expire",
		attribute.String("key", key),
		attribute.Int64("ttl_ms", expiration.Milliseconds()))
	defer span.End()

	ok, err := c.rdb.Expire(ctx, key, expiration).Result()
	if err != nil {
		span.RecordError(err)
		span.SetStatus(codes.Error, err.Error())
		return false, err
	}
	return ok, nil
}
