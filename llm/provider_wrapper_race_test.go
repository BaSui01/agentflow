package llm

import (
	"fmt"
	"sync"
	"testing"
)

// TestDefaultProviderFactory_ConcurrentRegisterAndCreate verifies that
// concurrent RegisterProvider and CreateProvider calls do not race.
// Run with: go test -race -run TestDefaultProviderFactory_ConcurrentRegisterAndCreate
func TestDefaultProviderFactory_ConcurrentRegisterAndCreate(t *testing.T) {
	t.Parallel()

	factory := NewDefaultProviderFactory()
	const goroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers: register providers concurrently
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				code := fmt.Sprintf("provider-%d-%d", id, i)
				factory.RegisterProvider(code, func(apiKey, baseURL string) (Provider, error) {
					return nil, nil
				})
			}
		}(g)
	}

	// Readers: create providers concurrently (some will fail with "not registered", that's fine)
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				code := fmt.Sprintf("provider-%d-%d", id, i)
				factory.CreateProvider(code, "key", "url")
			}
		}(g)
	}

	wg.Wait()
}

// TestDefaultProviderFactory_ConcurrentRegisterOnly verifies that many
// goroutines can register providers simultaneously without a race.
func TestDefaultProviderFactory_ConcurrentRegisterOnly(t *testing.T) {
	t.Parallel()

	factory := NewDefaultProviderFactory()
	const goroutines = 100

	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			code := fmt.Sprintf("p-%d", id)
			factory.RegisterProvider(code, func(apiKey, baseURL string) (Provider, error) {
				return nil, nil
			})
		}(g)
	}

	wg.Wait()

	// Verify all registrations succeeded
	for g := 0; g < goroutines; g++ {
		code := fmt.Sprintf("p-%d", g)
		_, err := factory.CreateProvider(code, "k", "u")
		if err != nil {
			t.Errorf("provider %s should be registered: %v", code, err)
		}
	}
}
