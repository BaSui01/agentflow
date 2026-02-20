package retry

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
)

func TestBackoffRetryer_Success(t *testing.T) {
	logger := zap.NewNop()
	policy := &RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	retryer := NewBackoffRetryer(policy, logger)
	ctx := context.Background()

	callCount := 0
	err := retryer.Do(ctx, func() error {
		callCount++
		return nil // 第一次就成功
	})

	assert.NoError(t, err)
	assert.Equal(t, 1, callCount, "应该只调用一次")
}

func TestBackoffRetryer_RetryAndSuccess(t *testing.T) {
	logger := zap.NewNop()
	policy := &RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	retryer := NewBackoffRetryer(policy, logger)
	ctx := context.Background()

	callCount := 0
	testErr := errors.New("temporary error")

	err := retryer.Do(ctx, func() error {
		callCount++
		if callCount < 3 {
			return testErr // 前两次失败
		}
		return nil // 第三次成功
	})

	assert.NoError(t, err)
	assert.Equal(t, 3, callCount, "应该调用三次")
}

func TestBackoffRetryer_MaxRetriesExceeded(t *testing.T) {
	logger := zap.NewNop()
	policy := &RetryPolicy{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}

	retryer := NewBackoffRetryer(policy, logger)
	ctx := context.Background()

	callCount := 0
	testErr := errors.New("persistent error")

	err := retryer.Do(ctx, func() error {
		callCount++
		return testErr // 始终失败
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "重试 2 次后仍失败")
	assert.Equal(t, 3, callCount, "应该调用三次（初始+2次重试）")
}

func TestBackoffRetryer_ContextCanceled(t *testing.T) {
	logger := zap.NewNop()
	policy := &RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       false,
	}

	retryer := NewBackoffRetryer(policy, logger)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	callCount := 0
	testErr := errors.New("error")

	err := retryer.Do(ctx, func() error {
		callCount++
		return testErr
	})

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "重试被取消")
	assert.GreaterOrEqual(t, callCount, 1, "至少调用一次")
}

func TestBackoffRetryer_RetryableErrors(t *testing.T) {
	logger := zap.NewNop()

	retryableErr := errors.New("retryable error")
	nonRetryableErr := errors.New("non-retryable error")

	policy := &RetryPolicy{
		MaxRetries:      3,
		InitialDelay:    10 * time.Millisecond,
		MaxDelay:        100 * time.Millisecond,
		Multiplier:      2.0,
		Jitter:          false,
		RetryableErrors: []error{retryableErr},
	}

	retryer := NewBackoffRetryer(policy, logger)
	ctx := context.Background()

	// 测试可重试错误
	t.Run("retryable error", func(t *testing.T) {
		callCount := 0
		err := retryer.Do(ctx, func() error {
			callCount++
			if callCount < 3 {
				return retryableErr
			}
			return nil
		})

		assert.NoError(t, err)
		assert.Equal(t, 3, callCount)
	})

	// 测试不可重试错误
	t.Run("non-retryable error", func(t *testing.T) {
		callCount := 0
		err := retryer.Do(ctx, func() error {
			callCount++
			return nonRetryableErr
		})

		assert.Error(t, err)
		assert.Equal(t, 1, callCount, "不应该重试")
	})
}

func TestBackoffRetryer_DelayCalculation(t *testing.T) {
	logger := zap.NewNop()
	policy := &RetryPolicy{
		MaxRetries:   5,
		InitialDelay: 100 * time.Millisecond,
		MaxDelay:     1 * time.Second,
		Multiplier:   2.0,
		Jitter:       false,
	}

	retryer := NewBackoffRetryer(policy, logger).(*backoffRetryer)

	tests := []struct {
		attempt     int
		expectedMin time.Duration
		expectedMax time.Duration
	}{
		{1, 100 * time.Millisecond, 100 * time.Millisecond}, // 初始延迟
		{2, 200 * time.Millisecond, 200 * time.Millisecond}, // 100 * 2^1
		{3, 400 * time.Millisecond, 400 * time.Millisecond}, // 100 * 2^2
		{4, 800 * time.Millisecond, 800 * time.Millisecond}, // 100 * 2^3
		{5, 1 * time.Second, 1 * time.Second},               // 达到最大延迟
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			delay := retryer.calculateDelay(tt.attempt)
			assert.GreaterOrEqual(t, delay, tt.expectedMin)
			assert.LessOrEqual(t, delay, tt.expectedMax)
		})
	}
}

func TestBackoffRetryer_OnRetryCallback(t *testing.T) {
	logger := zap.NewNop()

	callbackCount := 0
	var lastAttempt int
	var lastErr error
	var lastDelay time.Duration

	policy := &RetryPolicy{
		MaxRetries:   2,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
		OnRetry: func(attempt int, err error, delay time.Duration) {
			callbackCount++
			lastAttempt = attempt
			lastErr = err
			lastDelay = delay
		},
	}

	retryer := NewBackoffRetryer(policy, logger)
	ctx := context.Background()

	testErr := errors.New("test error")
	callCount := 0

	_ = retryer.Do(ctx, func() error {
		callCount++
		if callCount < 3 {
			return testErr
		}
		return nil
	})

	assert.Equal(t, 2, callbackCount, "回调应该被调用两次")
	assert.Equal(t, 2, lastAttempt)
	assert.Equal(t, testErr, lastErr)
	assert.Greater(t, lastDelay, time.Duration(0))
}

func TestWrapRetryable(t *testing.T) {
	err := errors.New("test error")
	wrapped := WrapRetryable(err)

	assert.True(t, IsRetryable(wrapped))
	assert.False(t, IsRetryable(err))
}

// ---------------------------------------------------------------------------
// DoWithResultTyped (generic wrapper)
// ---------------------------------------------------------------------------

func TestDoWithResultTyped_Success(t *testing.T) {
	r := NewBackoffRetryer(&RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}, zap.NewNop())

	val, err := DoWithResultTyped[int](r, context.Background(), func() (int, error) {
		return 42, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 42, val)
}

func TestDoWithResultTyped_Error(t *testing.T) {
	r := NewBackoffRetryer(&RetryPolicy{
		MaxRetries:   0,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}, zap.NewNop())

	val, err := DoWithResultTyped[int](r, context.Background(), func() (int, error) {
		return 0, errors.New("fail")
	})
	assert.Error(t, err)
	assert.Equal(t, 0, val)
}

func TestDoWithResultTyped_RetryThenSuccess(t *testing.T) {
	r := NewBackoffRetryer(&RetryPolicy{
		MaxRetries:   3,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}, zap.NewNop())

	callCount := 0
	val, err := DoWithResultTyped[string](r, context.Background(), func() (string, error) {
		callCount++
		if callCount < 3 {
			return "", errors.New("not yet")
		}
		return "done", nil
	})
	assert.NoError(t, err)
	assert.Equal(t, "done", val)
	assert.Equal(t, 3, callCount)
}

func TestDoWithResultTyped_Struct(t *testing.T) {
	type result struct {
		Value int
	}

	r := NewBackoffRetryer(&RetryPolicy{
		MaxRetries:   1,
		InitialDelay: 10 * time.Millisecond,
		MaxDelay:     100 * time.Millisecond,
		Multiplier:   2.0,
		Jitter:       false,
	}, zap.NewNop())

	val, err := DoWithResultTyped[result](r, context.Background(), func() (result, error) {
		return result{Value: 100}, nil
	})
	assert.NoError(t, err)
	assert.Equal(t, 100, val.Value)
}
