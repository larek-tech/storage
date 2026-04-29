package redis

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cmdable описывает множество команд go-redis, используемых пакетом.
// Совместимо как с *redis.Client, так и с redis.Pipeliner / транзакциями.
type Cmdable interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value any, expiration time.Duration) *redis.StatusCmd
	Del(ctx context.Context, keys ...string) *redis.IntCmd
	Exists(ctx context.Context, keys ...string) *redis.IntCmd
	Expire(ctx context.Context, key string, expiration time.Duration) *redis.BoolCmd
	Ping(ctx context.Context) *redis.StatusCmd
}
