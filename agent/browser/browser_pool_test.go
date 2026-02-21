package browser

import (
	"context"
	"sync"
	"testing"

	"go.uber.org/zap"
)

// newTestBrowser creates a ChromeDPBrowser with a stub driver suitable for testing.
// The driver's Close() is safe to call (no-op cancel funcs).
func newTestBrowser() *ChromeDPBrowser {
	ctx, cancel := context.WithCancel(context.Background())
	driver := &ChromeDPDriver{
		ctx:         ctx,
		cancel:      cancel,
		allocCtx:    ctx,
		allocCancel: cancel,
		logger:      zap.NewNop(),
	}
	return &ChromeDPBrowser{
		driver: driver,
		config: DefaultBrowserConfig(),
		logger: zap.NewNop(),
	}
}

// TestReleaseAfterClose verifies that calling Release after Close does not panic.
func TestReleaseAfterClose(t *testing.T) {
	logger := zap.NewNop()

	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 2),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 2,
		minIdle: 0,
		logger:  logger,
	}

	fakeBrowser := newTestBrowser()
	pool.active[fakeBrowser] = true

	// Close the pool first.
	pool.Close()

	// Release after close must not panic (this was the original bug).
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Release panicked after Close: %v", r)
			}
		}()
		pool.Release(fakeBrowser)
	}()
}

// TestConcurrentReleaseAndClose runs Release and Close concurrently to verify no race/panic.
func TestConcurrentReleaseAndClose(t *testing.T) {
	logger := zap.NewNop()

	for iter := 0; iter < 50; iter++ {
		pool := &BrowserPool{
			config:  DefaultBrowserConfig(),
			pool:    make(chan *ChromeDPBrowser, 5),
			active:  make(map[*ChromeDPBrowser]bool),
			maxSize: 5,
			minIdle: 0,
			logger:  logger,
		}

		browsers := make([]*ChromeDPBrowser, 5)
		for i := range browsers {
			browsers[i] = newTestBrowser()
			pool.active[browsers[i]] = true
		}

		var wg sync.WaitGroup

		// Concurrently release all browsers.
		for _, b := range browsers {
			wg.Add(1)
			go func(br *ChromeDPBrowser) {
				defer func() {
					if r := recover(); r != nil {
						t.Errorf("Release panicked: %v", r)
					}
					wg.Done()
				}()
				pool.Release(br)
			}(b)
		}

		// Concurrently close the pool.
		wg.Add(1)
		go func() {
			defer func() {
				if r := recover(); r != nil {
					t.Errorf("Close panicked: %v", r)
				}
				wg.Done()
			}()
			pool.Close()
		}()

		wg.Wait()
	}
}

// TestReleaseToFullPool verifies that releasing to a full pool closes the excess browser.
func TestReleaseToFullPool(t *testing.T) {
	logger := zap.NewNop()

	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 1),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 1,
		minIdle: 0,
		logger:  logger,
	}

	// Fill the pool channel.
	occupant := newTestBrowser()
	pool.pool <- occupant

	// Release another browser -- pool is full, so it should go to the default branch.
	extra := newTestBrowser()
	pool.active[extra] = true

	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Fatalf("Release panicked on full pool: %v", r)
			}
		}()
		pool.Release(extra)
	}()

	// The active map should no longer contain the extra browser.
	pool.mu.Lock()
	if pool.active[extra] {
		t.Error("expected extra browser to be removed from active map")
	}
	pool.mu.Unlock()
}
