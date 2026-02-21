package agent

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// TestSimpleEventBus_ConcurrentSubscribePublish verifies that concurrent
// Subscribe, Unsubscribe, and Publish calls do not race on the handlers map.
// Run with: go test -race -run TestSimpleEventBus_ConcurrentSubscribePublish
func TestSimpleEventBus_ConcurrentSubscribePublish(t *testing.T) {
	t.Parallel()

	bus := NewEventBus(zap.NewNop())
	defer bus.Stop()

	const goroutines = 50
	const opsPerGoroutine = 50

	var wg sync.WaitGroup
	wg.Add(goroutines * 3) // subscribers + unsubscribers + publishers

	// Collect subscription IDs for unsubscription
	ids := make(chan string, goroutines*opsPerGoroutine)

	// Subscribers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				id := bus.Subscribe(EventToolCall, func(e Event) {})
				ids <- id
			}
		}()
	}

	// Unsubscribers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				select {
				case id := <-ids:
					bus.Unsubscribe(id)
				default:
				}
			}
		}()
	}

	// Publishers
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < opsPerGoroutine; i++ {
				bus.Publish(&ToolCallEvent{
					ToolName:   "test",
					Timestamp_: time.Now(),
				})
			}
		}()
	}

	wg.Wait()
}

// TestSimpleEventBus_HandlersCopiedBeforeIteration verifies that the
// processEvents loop works on a snapshot of handlers, so concurrent
// Subscribe/Unsubscribe during handler execution does not panic.
func TestSimpleEventBus_HandlersCopiedBeforeIteration(t *testing.T) {
	t.Parallel()

	bus := NewEventBus(zap.NewNop())
	defer bus.Stop()

	var called int64

	// Subscribe a handler that takes some time
	bus.Subscribe(EventStateChange, func(e Event) {
		atomic.AddInt64(&called, 1)
		time.Sleep(5 * time.Millisecond)
	})

	// Publish an event
	bus.Publish(&StateChangeEvent{
		AgentID_:   "test",
		FromState:  StateInit,
		ToState:    StateRunning,
		Timestamp_: time.Now(),
	})

	// While the handler is running, subscribe and unsubscribe rapidly
	var wg sync.WaitGroup
	wg.Add(20)
	for i := 0; i < 20; i++ {
		go func() {
			defer wg.Done()
			id := bus.Subscribe(EventStateChange, func(e Event) {})
			bus.Unsubscribe(id)
		}()
	}

	wg.Wait()

	// Give the handler time to complete
	time.Sleep(50 * time.Millisecond)

	if atomic.LoadInt64(&called) == 0 {
		t.Error("expected handler to be called at least once")
	}
}
