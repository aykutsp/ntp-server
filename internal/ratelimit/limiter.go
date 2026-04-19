package ratelimit

import (
	"hash/fnv"
	"sync"
	"time"
)

type TokenBucket struct {
	mu     sync.Mutex
	rate   float64
	burst  float64
	tokens float64
	last   time.Time
}

func NewTokenBucket(rate float64, burst int) *TokenBucket {
	if rate <= 0 || burst <= 0 {
		return nil
	}
	now := time.Now()
	return &TokenBucket{
		rate:   rate,
		burst:  float64(burst),
		tokens: float64(burst),
		last:   now,
	}
}

func (b *TokenBucket) Allow(now time.Time) bool {
	if b == nil {
		return true
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * b.rate
		if b.tokens > b.burst {
			b.tokens = b.burst
		}
	}
	b.last = now

	if b.tokens >= 1 {
		b.tokens -= 1
		return true
	}
	return false
}

type KeyedLimiter struct {
	shards []limiterShard
	rate   float64
	burst  float64
	ttl    time.Duration
}

type limiterShard struct {
	mu      sync.Mutex
	buckets map[string]bucket
}

type bucket struct {
	tokens  float64
	last    time.Time
	expires time.Time
}

func NewKeyedLimiter(rate float64, burst int, ttl time.Duration, shardCount int) *KeyedLimiter {
	if rate <= 0 || burst <= 0 {
		return nil
	}
	if ttl <= 0 {
		ttl = 10 * time.Minute
	}
	if shardCount <= 0 {
		shardCount = 64
	}
	shards := make([]limiterShard, shardCount)
	for i := range shards {
		shards[i].buckets = make(map[string]bucket)
	}
	return &KeyedLimiter{
		shards: shards,
		rate:   rate,
		burst:  float64(burst),
		ttl:    ttl,
	}
}

func (l *KeyedLimiter) Allow(key string, now time.Time) bool {
	if l == nil {
		return true
	}
	shard := &l.shards[l.shardIndex(key)]

	shard.mu.Lock()
	defer shard.mu.Unlock()

	b, ok := shard.buckets[key]
	if !ok {
		shard.buckets[key] = bucket{
			tokens:  l.burst - 1,
			last:    now,
			expires: now.Add(l.ttl),
		}
		return true
	}

	elapsed := now.Sub(b.last).Seconds()
	if elapsed > 0 {
		b.tokens += elapsed * l.rate
		if b.tokens > l.burst {
			b.tokens = l.burst
		}
	}
	b.last = now
	b.expires = now.Add(l.ttl)

	allowed := false
	if b.tokens >= 1 {
		b.tokens -= 1
		allowed = true
	}
	shard.buckets[key] = b

	if len(shard.buckets) > 10000 {
		l.compactShard(shard, now)
	}

	return allowed
}

func (l *KeyedLimiter) shardIndex(key string) uint32 {
	h := fnv.New32a()
	_, _ = h.Write([]byte(key))
	return h.Sum32() % uint32(len(l.shards))
}

func (l *KeyedLimiter) compactShard(shard *limiterShard, now time.Time) {
	for k, b := range shard.buckets {
		if now.After(b.expires) {
			delete(shard.buckets, k)
		}
	}
}
