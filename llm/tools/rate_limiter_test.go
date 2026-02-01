package tools

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucketLimiter_Basic(t *testing.T) {
	limiter := newTokenBucketLimiter(10, time.Second)

	// Should allow first 10 calls
	for i := 0; i < 10; i++ {
		err := limiter.Allow()
		if err != nil {
			t.Errorf("call %d should be allowed, got error: %v", i, err)
		}
	}

	// 11th call should be rejected
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error for 11th call")
	}
}

func TestTokenBucketLimiter_Refill(t *testing.T) {
	// 10 calls per 100ms = 100 calls per second
	limiter := newTokenBucketLimiter(10, 100*time.Millisecond)

	// Consume all tokens
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}

	// Should be rejected immediately
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error after consuming all tokens")
	}

	// Wait for tokens to refill (wait for ~2 tokens to refill)
	time.Sleep(25 * time.Millisecond)

	// Should have at least 1 token now
	err = limiter.Allow()
	if err != nil {
		t.Errorf("expected call to be allowed after refill, got error: %v", err)
	}
}

func TestTokenBucketLimiter_Reset(t *testing.T) {
	limiter := newTokenBucketLimiter(5, time.Second)

	// Consume all tokens
	for i := 0; i < 5; i++ {
		limiter.Allow()
	}

	// Should be rejected
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error")
	}

	// Reset
	limiter.Reset()

	// Should be allowed again
	err = limiter.Allow()
	if err != nil {
		t.Errorf("expected call to be allowed after reset, got error: %v", err)
	}
}

func TestTokenBucketLimiter_Tokens(t *testing.T) {
	limiter := newTokenBucketLimiter(10, time.Second)

	// Initial tokens should be max
	tokens := limiter.Tokens()
	if tokens != 10 {
		t.Errorf("expected 10 tokens, got %f", tokens)
	}

	// Consume one token
	limiter.Allow()

	// Should have 9 tokens
	tokens = limiter.Tokens()
	if tokens < 8.9 || tokens > 9.1 {
		t.Errorf("expected ~9 tokens, got %f", tokens)
	}
}

func TestTokenBucketLimiter_Concurrent(t *testing.T) {
	limiter := newTokenBucketLimiter(100, time.Second)

	var wg sync.WaitGroup
	var successCount int
	var mu sync.Mutex

	// Launch 200 concurrent requests
	for i := 0; i < 200; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			err := limiter.Allow()
			if err == nil {
				mu.Lock()
				successCount++
				mu.Unlock()
			}
		}()
	}

	wg.Wait()

	// Should have exactly 100 successful calls
	if successCount != 100 {
		t.Errorf("expected 100 successful calls, got %d", successCount)
	}
}

func TestRateLimiter_BackwardCompatibility(t *testing.T) {
	// Test that the old rateLimiter interface still works
	limiter := newRateLimiter(5, time.Second)

	// Should allow first 5 calls
	for i := 0; i < 5; i++ {
		err := limiter.Allow()
		if err != nil {
			t.Errorf("call %d should be allowed, got error: %v", i, err)
		}
	}

	// 6th call should be rejected
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error for 6th call")
	}
}

// Benchmark tests to compare performance
func BenchmarkTokenBucketLimiter_Allow(b *testing.B) {
	limiter := newTokenBucketLimiter(1000000, time.Second)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		limiter.Allow()
	}
}

func BenchmarkTokenBucketLimiter_AllowConcurrent(b *testing.B) {
	limiter := newTokenBucketLimiter(1000000, time.Second)

	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			limiter.Allow()
		}
	})
}
