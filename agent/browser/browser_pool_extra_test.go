package browser

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

// TestBrowserPool_NewBrowserPool_ZeroMinIdle tests pool creation with no pre-created browsers.
func TestBrowserPool_NewBrowserPool_ZeroMinIdle(t *testing.T) {
	cfg := BrowserPoolConfig{
		MaxSize:       3,
		MinIdle:       0,
		MaxIdleTime:   time.Minute,
		BrowserConfig: DefaultBrowserConfig(),
	}
	// NewBrowserPool with MinIdle=0 should not call NewChromeDPBrowser
	pool := &BrowserPool{
		config:  cfg.BrowserConfig,
		pool:    make(chan *ChromeDPBrowser, cfg.MaxSize),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: cfg.MaxSize,
		minIdle: cfg.MinIdle,
		logger:  zap.NewNop(),
	}
	require.NotNil(t, pool)

	idle, active, total := pool.Stats()
	assert.Equal(t, 0, idle)
	assert.Equal(t, 0, active)
	assert.Equal(t, 0, total)
}

// TestBrowserPool_Acquire_FromPool tests acquiring a browser from the pool channel.
func TestBrowserPool_Acquire_FromPool(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 3),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 3,
		minIdle: 0,
		logger:  zap.NewNop(),
	}

	// Pre-fill pool with a test browser
	browser := newTestBrowser()
	pool.pool <- browser

	acquired, err := pool.Acquire(context.Background())
	require.NoError(t, err)
	assert.Equal(t, browser, acquired)

	pool.mu.Lock()
	assert.True(t, pool.active[acquired])
	pool.mu.Unlock()
}

// TestBrowserPool_Acquire_PoolClosed tests acquiring from a closed pool.
func TestBrowserPool_Acquire_PoolClosed(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 3),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 3,
		minIdle: 0,
		logger:  zap.NewNop(),
		closed:  true,
	}

	_, err := pool.Acquire(context.Background())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "closed")
}

func TestBrowserPool_Acquire_ClosedChannelReturnsError(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 1),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 1,
		minIdle: 0,
		logger:  zap.NewNop(),
	}
	close(pool.pool)

	browser, err := pool.Acquire(context.Background())
	require.Error(t, err)
	assert.Nil(t, browser)
	assert.Contains(t, err.Error(), "closed")
}

// TestBrowserPool_Acquire_PoolExhausted_ContextCancelled tests timeout when pool is full.
func TestBrowserPool_Acquire_PoolExhausted_ContextCancelled(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 1),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 1,
		minIdle: 0,
		logger:  zap.NewNop(),
	}

	// Mark one active so totalCount == maxSize
	pool.active[newTestBrowser()] = true

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	_, err := pool.Acquire(ctx)
	assert.Error(t, err)
}

// TestBrowserPool_Stats tests the Stats method.
func TestBrowserPool_Stats(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 5),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 5,
		minIdle: 0,
		logger:  zap.NewNop(),
	}

	// Add 2 to pool, 1 active
	pool.pool <- newTestBrowser()
	pool.pool <- newTestBrowser()
	pool.active[newTestBrowser()] = true

	idle, active, total := pool.Stats()
	assert.Equal(t, 2, idle)
	assert.Equal(t, 1, active)
	assert.Equal(t, 3, total)
}

// TestBrowserPool_Release_Normal tests releasing a browser back to the pool.
func TestBrowserPool_Release_Normal(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 3),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 3,
		minIdle: 0,
		logger:  zap.NewNop(),
	}

	browser := newTestBrowser()
	pool.active[browser] = true

	pool.Release(browser)

	pool.mu.Lock()
	assert.False(t, pool.active[browser])
	pool.mu.Unlock()

	// Browser should be back in pool
	assert.Equal(t, 1, len(pool.pool))
}

// TestBrowserPool_Close_CleansUp tests that Close cleans up all browsers.
func TestBrowserPool_Close_CleansUp(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 3),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 3,
		minIdle: 0,
		logger:  zap.NewNop(),
	}

	pool.pool <- newTestBrowser()
	pool.active[newTestBrowser()] = true

	err := pool.Close()
	require.NoError(t, err)
	assert.True(t, pool.closed)
}

// TestBrowserPool_ConcurrentAcquireRelease tests concurrent acquire/release.
func TestBrowserPool_ConcurrentAcquireRelease(t *testing.T) {
	pool := &BrowserPool{
		config:  DefaultBrowserConfig(),
		pool:    make(chan *ChromeDPBrowser, 5),
		active:  make(map[*ChromeDPBrowser]bool),
		maxSize: 5,
		minIdle: 0,
		logger:  zap.NewNop(),
	}

	// Pre-fill pool
	for i := 0; i < 5; i++ {
		pool.pool <- newTestBrowser()
	}

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, err := pool.Acquire(context.Background())
			if err != nil {
				return
			}
			time.Sleep(time.Millisecond)
			pool.Release(b)
		}()
	}
	wg.Wait()

	pool.Close()
}

