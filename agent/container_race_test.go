package agent

import (
	"fmt"
	"sync"
	"testing"
)

// TestServiceLocator_ConcurrentRegisterAndGet verifies that concurrent
// Register and Get calls do not trigger a data race on the services map.
// Run with: go test -race -run TestServiceLocator_ConcurrentRegisterAndGet
func TestServiceLocator_ConcurrentRegisterAndGet(t *testing.T) {
	t.Parallel()

	sl := NewServiceLocator()
	const goroutines = 50
	const opsPerGoroutine = 100

	var wg sync.WaitGroup
	wg.Add(goroutines * 2) // writers + readers

	// Writers: concurrently register services
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				name := fmt.Sprintf("svc-%d-%d", id, i)
				sl.Register(name, i)
			}
		}(g)
	}

	// Readers: concurrently get services
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				name := fmt.Sprintf("svc-%d-%d", id, i)
				sl.Get(name)
			}
		}(g)
	}

	wg.Wait()
}

// TestServiceLocator_ConcurrentMustGet verifies MustGet under concurrency.
func TestServiceLocator_ConcurrentMustGet(t *testing.T) {
	t.Parallel()

	sl := NewServiceLocator()
	// Pre-register a service so MustGet won't panic
	sl.Register("shared", "value")

	const goroutines = 50
	var wg sync.WaitGroup
	wg.Add(goroutines)

	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < 100; i++ {
				v := sl.MustGet("shared")
				if v != "value" {
					t.Errorf("unexpected value: %v", v)
				}
			}
		}()
	}

	wg.Wait()
}

// TestServiceLocator_ConcurrentTypedGetters exercises all typed getter
// methods concurrently alongside Register to detect races.
func TestServiceLocator_ConcurrentTypedGetters(t *testing.T) {
	t.Parallel()

	sl := NewServiceLocator()
	const goroutines = 30

	var wg sync.WaitGroup
	wg.Add(goroutines * 2)

	// Writers
	for g := 0; g < goroutines; g++ {
		go func(id int) {
			defer wg.Done()
			key := fmt.Sprintf("key-%d", id)
			sl.Register(key, id)
		}(g)
	}

	// Readers calling typed getters (they won't find the right type, but
	// the point is to exercise the lock path without races)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			sl.GetProvider()
			sl.GetMemory()
			sl.GetToolManager()
			sl.GetEventBus()
			sl.GetLogger()
		}()
	}

	wg.Wait()
}
