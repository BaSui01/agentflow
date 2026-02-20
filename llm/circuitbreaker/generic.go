package circuitbreaker

import "context"

// CallWithResultTyped is a type-safe generic wrapper around CircuitBreaker.CallWithResult.
// It eliminates the need for type assertions on the return value.
//
// Usage:
//
//	val, err := circuitbreaker.CallWithResultTyped[int](cb, ctx, func() (int, error) {
//	    return 42, nil
//	})
func CallWithResultTyped[T any](cb CircuitBreaker, ctx context.Context, fn func() (T, error)) (T, error) {
	result, err := cb.CallWithResult(ctx, func() (any, error) {
		return fn()
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result.(T), nil
}
