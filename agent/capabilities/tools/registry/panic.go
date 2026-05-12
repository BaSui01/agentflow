package registry

import "fmt"

// RecoveredPanicToError converts a recovered panic value into an error.
func RecoveredPanicToError(v any) error {
	if err, ok := v.(error); ok {
		return err
	}
	return fmt.Errorf("panic: %v", v)
}
