// Rate limiting middleware with pluggable in-memory and Redis backends.
package middleware

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimitBackend abstracts distributed rate-limit storage. Errors are
// surfaced so protected routes can fail closed instead of silently bypassing
// limits when Redis is unavailable.
type RateLimitBackend interface {
	AllowContext(ctx context.Context, key string, rate int, window time.Duration) (bool, error)
}

// ─── In-Memory Backend (development / single process) ───

type memoryBackend struct {
	mu       sync.Mutex
	visitors map[string]*visitor
}

type visitor struct {
	count     int
	firstSeen time.Time
	lastSeen  time.Time
}

func NewMemoryBackend() RateLimitBackend {
	mb := &memoryBackend{visitors: make(map[string]*visitor)}
	go func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for range ticker.C {
			mb.cleanup()
		}
	}()
	return mb
}

func (mb *memoryBackend) Allow(key string, rate int, window time.Duration) bool {
	allowed, _ := mb.AllowContext(context.Background(), key, rate, window)
	return allowed
}

func (mb *memoryBackend) AllowContext(_ context.Context, key string, rate int, window time.Duration) (bool, error) {
	mb.mu.Lock()
	defer mb.mu.Unlock()

	v, exists := mb.visitors[key]
	now := time.Now()
	if !exists || now.Sub(v.firstSeen) >= window {
		mb.visitors[key] = &visitor{count: 1, firstSeen: now, lastSeen: now}
		return true, nil
	}
	v.lastSeen = now
	v.count++
	return v.count <= rate, nil
}

func (mb *memoryBackend) cleanup() {
	mb.mu.Lock()
	defer mb.mu.Unlock()
	threshold := time.Now().Add(-5 * time.Minute)
	for key, v := range mb.visitors {
		if v.lastSeen.Before(threshold) {
			delete(mb.visitors, key)
		}
	}
}

// ─── Redis Backend ───

type redisBackend struct{ client redis.UniversalClient }

func NewRedisBackend(client redis.UniversalClient) RateLimitBackend {
	return &redisBackend{client: client}
}

const rateLimitScript = `
local current = redis.call('INCR', KEYS[1])
if current == 1 then
  redis.call('PEXPIRE', KEYS[1], ARGV[2])
end
return current
`

func (rb *redisBackend) AllowContext(ctx context.Context, key string, rate int, window time.Duration) (bool, error) {
	if rb == nil || rb.client == nil {
		return false, errors.New("redis rate-limit backend is unavailable")
	}
	count, err := rb.client.Eval(ctx, rateLimitScript, []string{"licensehub:rl:" + key}, rate, window.Milliseconds()).Int64()
	if err != nil {
		return false, err
	}
	return count <= int64(rate), nil
}

var (
	backendMu      sync.RWMutex
	defaultBackend RateLimitBackend = NewMemoryBackend()
)

func SetRateLimitBackend(b RateLimitBackend) {
	if b == nil {
		return
	}
	backendMu.Lock()
	defaultBackend = b
	backendMu.Unlock()
}

func getRateLimitBackend() RateLimitBackend {
	backendMu.RLock()
	defer backendMu.RUnlock()
	return defaultBackend
}

func rateLimit(rate int, window time.Duration, keyFn func(*gin.Context) string, failClosed bool) gin.HandlerFunc {
	return func(c *gin.Context) {
		allowed, err := getRateLimitBackend().AllowContext(c.Request.Context(), keyFn(c), rate, window)
		if err != nil {
			if failClosed {
				abortWithError(c, http.StatusServiceUnavailable, "RATE_LIMIT_UNAVAILABLE", "authentication protection is temporarily unavailable")
				return
			}
			c.Next()
			return
		}
		if !allowed {
			abortWithError(c, http.StatusTooManyRequests, "RATE_LIMITED", "too many requests, please try again later")
			return
		}
		c.Next()
	}
}

func requestIdentity(c *gin.Context) string {
	if ak, exists := c.Get("api_key"); exists {
		if apiKey, ok := ak.(interface{ GetID() string }); ok {
			return "apikey:" + apiKey.GetID()
		}
	}
	return "ip:" + c.ClientIP()
}

func RateLimit(rate int, window time.Duration) gin.HandlerFunc {
	return rateLimit(rate, window, requestIdentity, false)
}

func RateLimitByIP(rate int, window time.Duration) gin.HandlerFunc {
	return rateLimit(rate, window, func(c *gin.Context) string { return "ip:" + c.ClientIP() }, false)
}

// RateLimitByIPFailClosed is for authentication and administration routes.
// Redis failure becomes 503 rather than an unprotected request.
func RateLimitByIPFailClosed(rate int, window time.Duration) gin.HandlerFunc {
	return rateLimit(rate, window, func(c *gin.Context) string { return "ip:" + c.ClientIP() }, true)
}
