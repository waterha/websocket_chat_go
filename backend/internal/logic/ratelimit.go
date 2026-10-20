package logic

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"
)

// Limiter is a fixed-window counter used to slow down credential guessing. It
// lives in Redis so several processes would share it if the deployment ever
// scaled out, even though broadcasting currently cannot.
type Limiter struct{ Redis *redis.Client }

// limitKey hashes the subject so raw email addresses are not stored in Redis.
func limitKey(scope, subject string) string {
	sum := sha256.Sum256([]byte(subject))
	return "limit:" + scope + ":" + hex.EncodeToString(sum[:])
}

// Exceeded reports whether the subject has already used up its allowance, and
// how long the caller should wait. A Redis failure fails open: the alternative
// would let an unavailable cache lock every user out of their own account.
func (limiter *Limiter) Exceeded(ctx context.Context, scope, subject string, max int) (bool, time.Duration) {
	if limiter == nil || limiter.Redis == nil || max < 1 {
		return false, 0
	}
	key := limitKey(scope, subject)
	value, err := limiter.Redis.Get(ctx, key).Int()
	if err == redis.Nil {
		return false, 0
	}
	if err != nil {
		return false, 0
	}
	if value < max {
		return false, 0
	}
	ttl, err := limiter.Redis.TTL(ctx, key).Result()
	if err != nil || ttl < 0 {
		ttl = time.Minute
	}
	return true, ttl
}

// Record counts one failed attempt and sets the window on the first one. It is
// intentionally best-effort: a Redis error must not turn a wrong password into a
// server error.
func (limiter *Limiter) Record(ctx context.Context, scope, subject string, window time.Duration) {
	if limiter == nil || limiter.Redis == nil {
		return
	}
	key := limitKey(scope, subject)
	pipe := limiter.Redis.TxPipeline()
	count := pipe.Incr(ctx, key)
	pipe.ExpireNX(ctx, key, window)
	_, _ = pipe.Exec(ctx)
	_ = count
}

// Clear removes the allowance after a successful authentication.
func (limiter *Limiter) Clear(ctx context.Context, scope, subject string) {
	if limiter == nil || limiter.Redis == nil {
		return
	}
	_ = limiter.Redis.Del(ctx, limitKey(scope, subject)).Err()
}

// RetrySeconds renders a TTL for a Retry-After header value.
func RetrySeconds(ttl time.Duration) string {
	seconds := int(ttl.Seconds())
	if seconds < 1 {
		seconds = 1
	}
	return strconv.Itoa(seconds)
}
