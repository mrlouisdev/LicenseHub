package middleware

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// BruteForceProtection tracks failed authentication attempts and blocks
// IPs/keys that exceed the threshold. Uses exponential backoff.
type BruteForceProtection struct {
	mu         sync.RWMutex
	attempts   map[string]*attemptRecord
	maxFails   int           // max failures before lockout
	lockout    time.Duration // initial lockout duration
	maxLockout time.Duration // maximum lockout (cap for exponential backoff)
	window     time.Duration // window to count failures
	redis      redis.UniversalClient
}

type attemptRecord struct {
	failures     int
	firstFailure time.Time
	lockedUntil  time.Time
	lockoutCount int // number of times locked out (for exponential backoff)
}

func NewBruteForceProtection(maxFails int, lockout, maxLockout, window time.Duration) *BruteForceProtection {
	bf := &BruteForceProtection{
		attempts:   make(map[string]*attemptRecord),
		maxFails:   maxFails,
		lockout:    lockout,
		maxLockout: maxLockout,
		window:     window,
	}
	go bf.cleanup()
	return bf
}

// NewRedisBruteForceProtection stores lockout state in Redis so it survives
// process restarts and is shared by every replica. Keys are SHA-256 digests;
// raw IP addresses and license values are never stored in the Redis keyspace.
func NewRedisBruteForceProtection(client redis.UniversalClient, maxFails int, lockout, maxLockout, window time.Duration) *BruteForceProtection {
	bf := NewBruteForceProtection(maxFails, lockout, maxLockout, window)
	bf.redis = client
	return bf
}

func bruteForceStateKey(key string) string {
	sum := sha256.Sum256([]byte(key))
	return "licensehub:bf:" + hex.EncodeToString(sum[:])
}

const recordFailureScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local max_fails = tonumber(ARGV[1])
local lockout = tonumber(ARGV[2])
local max_lockout = tonumber(ARGV[3])
local window = tonumber(ARGV[4])
local failures = tonumber(redis.call('HGET', KEYS[1], 'failures') or '0')
local first = tonumber(redis.call('HGET', KEYS[1], 'first_failure') or '0')
local lockouts = tonumber(redis.call('HGET', KEYS[1], 'lockout_count') or '0')
local locked_until = tonumber(redis.call('HGET', KEYS[1], 'locked_until') or '0')
if first == 0 or now - first > window then
  failures = 0
  first = now
  lockouts = 0
end
failures = failures + 1
if failures >= max_fails then
  local duration = lockout
  for _ = 1, lockouts do
    duration = duration * 2
    if duration >= max_lockout then
      duration = max_lockout
      break
    end
  end
  if duration > max_lockout then duration = max_lockout end
  locked_until = now + duration
  lockouts = lockouts + 1
  failures = 0
  first = now
end
redis.call('HSET', KEYS[1],
  'failures', failures,
  'first_failure', first,
  'lockout_count', lockouts,
  'locked_until', locked_until)
local ttl = math.max(window * 4, max_lockout * 2)
redis.call('PEXPIRE', KEYS[1], ttl)
return locked_until
`

const blockedScript = `
local clock = redis.call('TIME')
local now = tonumber(clock[1]) * 1000 + math.floor(tonumber(clock[2]) / 1000)
local locked_until = tonumber(redis.call('HGET', KEYS[1], 'locked_until') or '0')
if locked_until > now then return locked_until - now end
return 0
`

func (bf *BruteForceProtection) redisContext() (context.Context, context.CancelFunc) {
	return context.WithTimeout(context.Background(), 1500*time.Millisecond)
}

// RecordFailure records a failed attempt for a key (IP or license key).
func (bf *BruteForceProtection) RecordFailure(key string) {
	key = bruteForceStateKey(key)
	if bf.redis != nil {
		ctx, cancel := bf.redisContext()
		defer cancel()
		_, _ = bf.redis.Eval(ctx, recordFailureScript, []string{key},
			bf.maxFails, bf.lockout.Milliseconds(), bf.maxLockout.Milliseconds(), bf.window.Milliseconds()).Result()
		return
	}
	bf.mu.Lock()
	defer bf.mu.Unlock()

	now := time.Now()
	rec, exists := bf.attempts[key]
	if !exists || now.Sub(rec.firstFailure) > bf.window {
		bf.attempts[key] = &attemptRecord{failures: 1, firstFailure: now}
		return
	}

	rec.failures++
	if rec.failures >= bf.maxFails {
		// Exponential backoff: lockout * 2^lockoutCount, capped at maxLockout
		duration := bf.lockout
		for i := 0; i < rec.lockoutCount; i++ {
			duration *= 2
			if duration > bf.maxLockout {
				duration = bf.maxLockout
				break
			}
		}
		rec.lockedUntil = now.Add(duration)
		rec.lockoutCount++
		rec.failures = 0
		rec.firstFailure = now
	}
}

// RecordSuccess clears the failure record for a key.
func (bf *BruteForceProtection) RecordSuccess(key string) {
	key = bruteForceStateKey(key)
	if bf.redis != nil {
		ctx, cancel := bf.redisContext()
		defer cancel()
		_ = bf.redis.Del(ctx, key).Err()
		return
	}
	bf.mu.Lock()
	defer bf.mu.Unlock()
	delete(bf.attempts, key)
}

// IsBlocked checks if a key is currently locked out.
func (bf *BruteForceProtection) IsBlocked(key string) (bool, time.Duration) {
	key = bruteForceStateKey(key)
	if bf.redis != nil {
		ctx, cancel := bf.redisContext()
		defer cancel()
		remaining, err := bf.redis.Eval(ctx, blockedScript, []string{key}).Int64()
		if err != nil {
			// Protected authentication routes fail closed if their distributed
			// state cannot be checked. Rate limiting uses the same policy.
			return true, 5 * time.Second
		}
		return remaining > 0, time.Duration(remaining) * time.Millisecond
	}
	bf.mu.RLock()
	defer bf.mu.RUnlock()

	rec, exists := bf.attempts[key]
	if !exists {
		return false, 0
	}

	now := time.Now()
	if rec.lockedUntil.After(now) {
		return true, rec.lockedUntil.Sub(now)
	}
	return false, 0
}

func (bf *BruteForceProtection) cleanup() {
	for {
		time.Sleep(5 * time.Minute)
		bf.mu.Lock()
		now := time.Now()
		for key, rec := range bf.attempts {
			// Remove records that are expired and not locked
			if now.Sub(rec.firstFailure) > bf.window*4 && !rec.lockedUntil.After(now) {
				delete(bf.attempts, key)
			}
		}
		bf.mu.Unlock()
	}
}

// LicenseBruteForceGuard is a middleware that checks brute-force state before processing.
func LicenseBruteForceGuard(bf *BruteForceProtection) gin.HandlerFunc {
	return func(c *gin.Context) {
		ip := "ip:" + c.ClientIP()
		blocked, retryAfter := bf.IsBlocked(ip)
		if blocked {
			BruteForceBlocks.Inc()
			// retry_after lives in `error.details` (NOT alongside
			// code/message) so the response shape stays the canonical
			// envelope: { success, error: { code, message, details? } }.
			retrySec := int(retryAfter.Seconds())
			c.Header("Retry-After", fmt.Sprintf("%d", retrySec))
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"success": false,
				"error": gin.H{
					"code":    "LOCKED_OUT",
					"message": "too many failed attempts, please try again later",
					"details": gin.H{"retry_after": retrySec},
				},
			})
			return
		}

		// Also check by license key if present in request body
		// We'll check post-processing via the context
		c.Set("brute_force", bf)
		c.Next()
	}
}

// RecordBruteForceFailure records a failure against the guard installed on
// the current route. Keeping this lookup in middleware prevents handlers from
// depending on the concrete storage implementation.
func RecordBruteForceFailure(c *gin.Context, key string) {
	if value, ok := c.Get("brute_force"); ok {
		if recorder, ok := value.(interface{ RecordFailure(string) }); ok {
			recorder.RecordFailure(key)
		}
	}
}

// RecordBruteForceSuccess clears the matching failure lineage after a valid
// authentication attempt.
func RecordBruteForceSuccess(c *gin.Context, key string) {
	if value, ok := c.Get("brute_force"); ok {
		if recorder, ok := value.(interface{ RecordSuccess(string) }); ok {
			recorder.RecordSuccess(key)
		}
	}
}
