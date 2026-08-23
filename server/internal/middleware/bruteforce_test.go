package middleware

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
)

func TestBruteForceMemoryLockoutAndClear(t *testing.T) {
	bf := NewBruteForceProtection(2, time.Minute, 10*time.Minute, 5*time.Minute)
	identity := "ip:192.0.2.10"
	bf.RecordFailure(identity)
	if blocked, _ := bf.IsBlocked(identity); blocked {
		t.Fatal("first failure must not lock the identity")
	}
	bf.RecordFailure(identity)
	if blocked, _ := bf.IsBlocked(identity); !blocked {
		t.Fatal("threshold failure must lock the identity")
	}
	bf.RecordSuccess(identity)
	if blocked, _ := bf.IsBlocked(identity); blocked {
		t.Fatal("successful authentication must clear the lockout")
	}
}

func TestBruteForceStateKeyDoesNotContainRawIdentity(t *testing.T) {
	raw := "key:LH-FIXTURE-VALUE"
	key := bruteForceStateKey(raw)
	if strings.Contains(key, raw) || strings.Contains(key, "FIXTURE") {
		t.Fatalf("distributed state key contains raw identity: %q", key)
	}
	if !strings.HasPrefix(key, "licensehub:bf:") {
		t.Fatalf("unexpected namespace: %q", key)
	}
}

func TestRedisBruteForcePersistsAcrossClients(t *testing.T) {
	url := os.Getenv("TEST_REDIS_URL")
	if url == "" {
		t.Skip("TEST_REDIS_URL not set")
	}
	opts, err := redis.ParseURL(url)
	if err != nil {
		t.Fatalf("parse TEST_REDIS_URL: %v", err)
	}
	identity := "ip:198.51.100.77:" + time.Now().UTC().Format(time.RFC3339Nano)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()

	client1 := redis.NewClient(opts)
	if err := client1.Ping(ctx).Err(); err != nil {
		client1.Close()
		t.Fatalf("ping first Redis client: %v", err)
	}
	bf1 := NewRedisBruteForceProtection(client1, 2, time.Minute, 10*time.Minute, 5*time.Minute)
	bf1.RecordFailure(identity)
	bf1.RecordFailure(identity)
	if err := client1.Close(); err != nil {
		t.Fatalf("close first Redis client: %v", err)
	}

	client2 := redis.NewClient(opts)
	defer client2.Close()
	if err := client2.Ping(ctx).Err(); err != nil {
		t.Fatalf("ping second Redis client: %v", err)
	}
	bf2 := NewRedisBruteForceProtection(client2, 2, time.Minute, 10*time.Minute, 5*time.Minute)
	if blocked, _ := bf2.IsBlocked(identity); !blocked {
		t.Fatal("lockout did not persist across clients")
	}
	bf2.RecordSuccess(identity)
	if blocked, _ := bf2.IsBlocked(identity); blocked {
		t.Fatal("cleared lockout remained blocked")
	}
}
