package whatsapp

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// Default rate limits guard the single shared WABA: a careless merchant must
// not be able to consume the platform's whole sending capacity.
const (
	defaultPerShopLimit = 120 // messages per minute per shop
	defaultGlobalLimit  = 600 // messages per minute across all shops
	defaultWindow       = time.Minute
)

// RateLimitExceeded is returned when a merchant or the platform exceeds the
// configured per-window budget.
type RateLimitExceeded struct {
	PerShop bool
}

func (e *RateLimitExceeded) Error() string {
	if e.PerShop {
		return "per-shop whatsapp rate limit exceeded"
	}
	return "platform whatsapp rate limit exceeded"
}

// RateLimiter is a fixed-window counter over Redis. Both the per-shop and the
// global window must have headroom for a send to pass.
type RateLimiter struct {
	rd      *redis.Client
	perShop int
	global  int
	window  time.Duration
	script  *redis.Script
}

// NewRateLimiter builds a limiter on an existing redis client. Nil client
// yields a no-op limiter so local/test setups without Redis stay functional.
func NewRateLimiter(rd *redis.Client) *RateLimiter {
	return &RateLimiter{
		rd:      rd,
		perShop: defaultPerShopLimit,
		global:  defaultGlobalLimit,
		window:  defaultWindow,
		script: redis.NewScript(`
			local sc = redis.call('INCR', KEYS[1])
			if sc == 1 then
				redis.call('EXPIRE', KEYS[1], ARGV[1])
			end
			local gc = redis.call('INCR', KEYS[2])
			if gc == 1 then
				redis.call('EXPIRE', KEYS[2], ARGV[1])
			end
			return {sc, gc}
		`),
	}
}

// Allow checks and increments both windows atomically. It returns
// *RateLimitExceeded when either budget is exhausted.
func (l *RateLimiter) Allow(ctx context.Context, shopID uuid.UUID) error {
	if l.rd == nil {
		return nil
	}
	windowSecs := int(l.window.Seconds())
	shopKey := fmt.Sprintf("rl:wa:shop:%s", shopID)
	globalKey := "rl:wa:global"

	counts, err := l.script.Run(ctx, l.rd,
		[]string{shopKey, globalKey}, windowSecs).Int64Slice()
	if err != nil {
		return err
	}
	if len(counts) != 2 {
		return nil
	}
	if counts[0] > int64(l.perShop) {
		return &RateLimitExceeded{PerShop: true}
	}
	if counts[1] > int64(l.global) {
		return &RateLimitExceeded{PerShop: false}
	}
	return nil
}
