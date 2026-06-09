package circuitbreaker

import (
	"context"
	"fmt"
)

// CallWithResultTyped is a type-safe generic wrapper around CircuitBreaker.CallWithResult.
// It eliminates the need for type assertions on the return value.
//
// Usage:
//
//	val, err := circuitbreaker.CallWithResultTyped[int](cb, ctx, func() (int, error) {
//	    return 42, nil
//	})
func CallWithResultTyped[T any](cb CircuitBreaker, ctx context.Context, fn func(context.Context) (T, error)) (T, error) {
	result, err := cb.CallWithResult(ctx, func(callCtx context.Context) (any, error) {
		return fn(callCtx)
	})
	if err != nil {
		var zero T
		return zero, err
	}
	if result == nil {
		var zero T
		return zero, nil
	}
	val, ok := result.(T)
	if !ok {
		var zero T
		return zero, fmt.Errorf("circuitbreaker: type assertion failed, expected %T, got %T", zero, result)
	}
	return val, nil
}
