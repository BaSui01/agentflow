// Package pool provides goroutine pool for controlled concurrency.
package pool

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrPoolClosed  = errors.New("pool is closed")
	ErrPoolFull    = errors.New("pool is full")
	ErrTaskTimeout = errors.New("task submission timeout")
)

// Task represents a unit of work.
type Task func(ctx context.Context) error

// GoroutinePool manages a pool of worker goroutines.
type GoroutinePool struct {
	maxWorkers  int
	taskQueue   chan taskWrapper
	workerCount atomic.Int32
	activeCount atomic.Int32
	closed      atomic.Bool
	wg          sync.WaitGroup

	// Metrics
	submitted atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64
	rejected  atomic.Int64

	// Config
	idleTimeout  time.Duration
	panicHandler func(any)
}

type taskWrapper struct {
	task   Task
	ctx    context.Context
	result chan error
}

// GoroutinePoolConfig configures the pool.
type GoroutinePoolConfig struct {
	MaxWorkers   int           `json:"max_workers"`
	QueueSize    int           `json:"queue_size"`
	IdleTimeout  time.Duration `json:"idle_timeout"`
	PanicHandler func(any)     `json:"-"`
}

// DefaultGoroutinePoolConfig returns sensible defaults.
func DefaultGoroutinePoolConfig() GoroutinePoolConfig {
	return GoroutinePoolConfig{
		MaxWorkers:  100,
		QueueSize:   1000,
		IdleTimeout: 60 * time.Second,
	}
}

// NewGoroutinePool creates a new goroutine pool.
func NewGoroutinePool(config GoroutinePoolConfig) *GoroutinePool {
	p := &GoroutinePool{
		maxWorkers:   config.MaxWorkers,
		taskQueue:    make(chan taskWrapper, config.QueueSize),
		idleTimeout:  config.IdleTimeout,
		panicHandler: config.PanicHandler,
	}
	return p
}

// Submit submits a task to the pool.
func (p *GoroutinePool) Submit(ctx context.Context, task Task) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	p.submitted.Add(1)

	wrapper := taskWrapper{
		task:   task,
		ctx:    ctx,
		result: make(chan error, 1),
	}

	// Try to submit to queue
	select {
	case p.taskQueue <- wrapper:
		p.ensureWorker()
		return nil
	default:
		// Queue full, try to spawn new worker
		if p.trySpawnWorker() {
			select {
			case p.taskQueue <- wrapper:
				return nil
			default:
			}
		}
		p.rejected.Add(1)
		return ErrPoolFull
	}
}

// SubmitWait submits a task and waits for completion.
func (p *GoroutinePool) SubmitWait(ctx context.Context, task Task) error {
	if p.closed.Load() {
		return ErrPoolClosed
	}

	p.submitted.Add(1)

	wrapper := taskWrapper{
		task:   task,
		ctx:    ctx,
		result: make(chan error, 1),
	}

	select {
	case p.taskQueue <- wrapper:
		p.ensureWorker()
	case <-ctx.Done():
		p.rejected.Add(1)
		return ctx.Err()
	}

	select {
	case err := <-wrapper.result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (p *GoroutinePool) ensureWorker() {
	if p.workerCount.Load() < int32(p.maxWorkers) {
		p.trySpawnWorker()
	}
}

func (p *GoroutinePool) trySpawnWorker() bool {
	for {
		current := p.workerCount.Load()
		if current >= int32(p.maxWorkers) {
			return false
		}
		if p.workerCount.CompareAndSwap(current, current+1) {
			p.wg.Add(1)
			go p.worker()
			return true
		}
	}
}

func (p *GoroutinePool) worker() {
	defer p.wg.Done()
	defer p.workerCount.Add(-1)

	timer := time.NewTimer(p.idleTimeout)
	defer timer.Stop()

	for {
		select {
		case wrapper, ok := <-p.taskQueue:
			if !ok {
				return
			}

			p.activeCount.Add(1)
			err := p.executeTask(wrapper)
			p.activeCount.Add(-1)

			if wrapper.result != nil {
				wrapper.result <- err
				close(wrapper.result)
			}

			if err != nil {
				p.failed.Add(1)
			} else {
				p.completed.Add(1)
			}

			timer.Reset(p.idleTimeout)

		case <-timer.C:
			// Idle timeout, exit if we have more than minimum workers
			if p.workerCount.Load() > 1 {
				return
			}
			timer.Reset(p.idleTimeout)
		}
	}
}

func (p *GoroutinePool) executeTask(wrapper taskWrapper) (err error) {
	defer func() {
		if r := recover(); r != nil {
			if p.panicHandler != nil {
				p.panicHandler(r)
			}
			err = errors.New("task panicked")
		}
	}()

	return wrapper.task(wrapper.ctx)
}

// Close closes the pool and waits for all workers to finish.
func (p *GoroutinePool) Close() {
	if p.closed.Swap(true) {
		return
	}
	close(p.taskQueue)
	p.wg.Wait()
}

// Stats returns pool statistics.
func (p *GoroutinePool) Stats() GoroutinePoolStats {
	return GoroutinePoolStats{
		Workers:   int(p.workerCount.Load()),
		Active:    int(p.activeCount.Load()),
		Queued:    len(p.taskQueue),
		Submitted: p.submitted.Load(),
		Completed: p.completed.Load(),
		Failed:    p.failed.Load(),
		Rejected:  p.rejected.Load(),
	}
}

// GoroutinePoolStats contains pool statistics.
type GoroutinePoolStats struct {
	Workers   int   `json:"workers"`
	Active    int   `json:"active"`
	Queued    int   `json:"queued"`
	Submitted int64 `json:"submitted"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	Rejected  int64 `json:"rejected"`
}
