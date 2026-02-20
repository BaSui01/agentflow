package retry

import "context"

// DoWithResultTyped is a type-safe generic wrapper around Retryer.DoWithResult.
// It eliminates the need for type assertions on the return value.
//
// Usage:
//
//	val, err := retry.DoWithResultTyped[int](r, ctx, func() (int, error) {
//	    return 42, nil
//	})
func DoWithResultTyped[T any](r Retryer, ctx context.Context, fn func() (T, error)) (T, error) {
	result, err := r.DoWithResult(ctx, func() (any, error) {
		return fn()
	})
	if err != nil {
		var zero T
		return zero, err
	}
	return result.(T), nil
}
