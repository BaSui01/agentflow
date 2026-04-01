package channelstore

import (
	"context"
	"sync"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

const (
	defaultRecordQueueSize = 2048
	defaultRecordWorkers   = 2
)

// AsyncUsageRecorderConfig controls the async recorder behavior.
type AsyncUsageRecorderConfig struct {
	// QueueSize is the buffered channel capacity. Default: 2048.
	QueueSize int

	// Workers is the number of background goroutines consuming the queue.
	// Default: 2.
	Workers int
}

func (c AsyncUsageRecorderConfig) withDefaults() AsyncUsageRecorderConfig {
	if c.QueueSize <= 0 {
		c.QueueSize = defaultRecordQueueSize
	}
	if c.Workers <= 0 {
		c.Workers = defaultRecordWorkers
	}
	return c
}

// UsageRecordSink is the synchronous backend that actually persists usage
// records (e.g., database write, metrics push). The AsyncUsageRecorder
// wraps this with an async queue.
type UsageRecordSink interface {
	PersistUsage(ctx context.Context, usage *router.ChannelUsageRecord) error
}

// AsyncUsageRecorder wraps a synchronous UsageRecordSink with an async
// worker-pool queue. Records are buffered and written in the background,
// preventing usage recording from blocking the critical request path.
type AsyncUsageRecorder struct {
	sink   UsageRecordSink
	queue  chan *router.ChannelUsageRecord
	stop   chan struct{}
	wg     sync.WaitGroup
	once   sync.Once
	mu     sync.RWMutex
	closed bool
}

var _ router.UsageRecorder = (*AsyncUsageRecorder)(nil)

// NewAsyncUsageRecorder creates and starts an async usage recorder.
func NewAsyncUsageRecorder(sink UsageRecordSink, cfg AsyncUsageRecorderConfig) *AsyncUsageRecorder {
	cfg = cfg.withDefaults()
	r := &AsyncUsageRecorder{
		sink:  sink,
		queue: make(chan *router.ChannelUsageRecord, cfg.QueueSize),
		stop:  make(chan struct{}),
	}
	for i := 0; i < cfg.Workers; i++ {
		r.wg.Add(1)
		go r.worker()
	}
	return r
}

// RecordUsage enqueues a usage record for async persistence.
// If the queue is full, the record is persisted synchronously as a fallback.
func (r *AsyncUsageRecorder) RecordUsage(ctx context.Context, usage *router.ChannelUsageRecord) error {
	if r == nil || usage == nil {
		return nil
	}

	r.mu.RLock()
	if r.closed {
		r.mu.RUnlock()
		return nil
	}

	select {
	case r.queue <- usage:
		r.mu.RUnlock()
		return nil
	default:
		r.mu.RUnlock()
		// Queue full — fallback to sync write to avoid data loss
		if r.sink != nil {
			return r.sink.PersistUsage(ctx, usage)
		}
		return nil
	}
}

func (r *AsyncUsageRecorder) worker() {
	defer r.wg.Done()
	for {
		select {
		case record := <-r.queue:
			if r.sink != nil && record != nil {
				_ = r.sink.PersistUsage(context.Background(), record)
			}
		case <-r.stop:
			// Drain remaining records
			for {
				select {
				case record := <-r.queue:
					if r.sink != nil && record != nil {
						_ = r.sink.PersistUsage(context.Background(), record)
					}
				default:
					return
				}
			}
		}
	}
}

// Shutdown stops the workers and drains the remaining queue.
// Blocks until all in-flight records are flushed or ctx is cancelled.
func (r *AsyncUsageRecorder) Shutdown(ctx context.Context) error {
	if r == nil {
		return nil
	}

	r.once.Do(func() {
		r.mu.Lock()
		r.closed = true
		close(r.stop)
		r.mu.Unlock()
	})

	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Pending returns the number of records waiting in the queue.
func (r *AsyncUsageRecorder) Pending() int {
	if r == nil {
		return 0
	}
	return len(r.queue)
}

// FuncSink adapts a plain function into a UsageRecordSink.
type FuncSink func(ctx context.Context, usage *router.ChannelUsageRecord) error

func (f FuncSink) PersistUsage(ctx context.Context, usage *router.ChannelUsageRecord) error {
	return f(ctx, usage)
}
