package idempotency

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// GetTyped is a type-safe generic wrapper around Manager.Get.
// It automatically unmarshals the cached JSON result into the target type T.
//
// Usage:
//
//	val, found, err := idempotency.GetTyped[MyResponse](m, ctx, key)
func GetTyped[T any](m Manager, ctx context.Context, key string) (T, bool, error) {
	var zero T
	raw, found, err := m.Get(ctx, key)
	if err != nil || !found {
		return zero, found, err
	}
	var result T
	if err := json.Unmarshal(raw, &result); err != nil {
		return zero, false, fmt.Errorf("unmarshal cached result: %w", err)
	}
	return result, true, nil
}

// SetTyped is a type-safe generic wrapper around Manager.Set.
// It accepts a typed value instead of any, providing compile-time type safety
// at the call site.
//
// Usage:
//
//	err := idempotency.SetTyped[MyResponse](m, ctx, key, resp, 10*time.Minute)
func SetTyped[T any](m Manager, ctx context.Context, key string, result T, ttl time.Duration) error {
	return m.Set(ctx, key, result, ttl)
}
