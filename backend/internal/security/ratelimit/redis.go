package ratelimit

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var fixedWindow = redis.NewScript(`
local count = redis.call('INCR', KEYS[1])
if count == 1 then redis.call('PEXPIRE', KEYS[1], ARGV[1]) end
return count
`)

type Limiter struct {
	client *redis.Client
	prefix string
}

func New(client *redis.Client, prefix string) *Limiter {
	return &Limiter{client: client, prefix: prefix}
}

func (l *Limiter) Allow(ctx context.Context, key string, limit int64, window time.Duration) (bool, error) {
	count, err := fixedWindow.Run(ctx, l.client, []string{l.prefix + key}, window.Milliseconds()).Int64()
	if err != nil {
		return false, fmt.Errorf("rate limit: %w", err)
	}
	return count <= limit, nil
}
