package tools

import (
	"sync"
	"testing"
	"time"
)

func TestTokenBucketLimiter_Basic(t *testing.T) {
	limiter := newTokenBucketLimiter(10, time.Second)

	// 应允许前十通电话
	for i := 0; i < 10; i++ {
		err := limiter.Allow()
		if err != nil {
			t.Errorf("call %d should be allowed, got error: %v", i, err)
		}
	}

	// 第11通电话应该拒绝
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error for 11th call")
	}
}

func TestTokenBucketLimiter_Refill(t *testing.T) {
	// 每100米打出10通电话=每秒打出100通电话
	limiter := newTokenBucketLimiter(10, 100*time.Millisecond)

	// 设定所有符牌
	for i := 0; i < 10; i++ {
		limiter.Allow()
	}

	// 应该马上驳回
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error after consuming all tokens")
	}

	// 等待重新填充符( 等待~ 2 个再填充符)
	time.Sleep(25 * time.Millisecond)

	// 现在至少应该有1个符号
	err = limiter.Allow()
	if err != nil {
		t.Errorf("expected call to be allowed after refill, got error: %v", err)
	}
}

func TestTokenBucketLimiter_Reset(t *testing.T) {
	limiter := newTokenBucketLimiter(5, time.Second)

	// 设定所有符牌
	for i := 0; i < 5; i++ {
		limiter.Allow()
	}

	// 应该拒绝
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error")
	}

	// 重设
	limiter.Reset()

	// 应该再允许一次
	err = limiter.Allow()
	if err != nil {
		t.Errorf("expected call to be allowed after reset, got error: %v", err)
	}
}

func TestTokenBucketLimiter_Tokens(t *testing.T) {
	limiter := newTokenBucketLimiter(10, time.Second)

	// 初始符号应为最大值
	tokens := limiter.Tokens()
	if tokens != 10 {
		t.Errorf("expected 10 tokens, got %f", tokens)
	}

	// 假设一个符号
	limiter.Allow()

	// 应该有9个纪念品
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

	// 发出200项同时提出的请求
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

	// 应该有100个成功的电话
	if successCount != 100 {
		t.Errorf("expected 100 successful calls, got %d", successCount)
	}
}

func TestRateLimiter_BackwardCompatibility(t *testing.T) {
	// 测试旧率Limiter 界面是否仍然有效
	limiter := newRateLimiter(5, time.Second)

	// 应允许先打五通电话
	for i := 0; i < 5; i++ {
		err := limiter.Allow()
		if err != nil {
			t.Errorf("call %d should be allowed, got error: %v", i, err)
		}
	}

	// 第六通电话应该拒绝
	err := limiter.Allow()
	if err == nil {
		t.Error("expected rate limit error for 6th call")
	}
}

// 比较业绩的基准测试
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
