// Package batch provides batch processing for LLM requests.
package batch

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"
	"time"
)

var (
	ErrBatchClosed  = errors.New("batch processor closed")
	ErrBatchTimeout = errors.New("batch timeout")
	ErrBatchFull    = errors.New("batch queue full")
)

// Request represents a single LLM request in a batch.
type Request struct {
	ID       string         `json:"id"`
	Model    string         `json:"model"`
	Messages []Message      `json:"messages"`
	Params   map[string]any `json:"params,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Response represents a single LLM response.
type Response struct {
	ID      string `json:"id"`
	Content string `json:"content"`
	Error   error  `json:"error,omitempty"`
	Tokens  int    `json:"tokens"`
}

// BatchHandler processes a batch of requests.
type BatchHandler func(ctx context.Context, requests []*Request) []*Response

// BatchConfig configures the batch processor.
type BatchConfig struct {
	MaxBatchSize   int           `json:"max_batch_size"`
	MaxWaitTime    time.Duration `json:"max_wait_time"`
	QueueSize      int           `json:"queue_size"`
	Workers        int           `json:"workers"`
	RetryOnFailure bool          `json:"retry_on_failure"`
}

// DefaultBatchConfig returns sensible defaults.
func DefaultBatchConfig() BatchConfig {
	return BatchConfig{
		MaxBatchSize:   10,
		MaxWaitTime:    100 * time.Millisecond,
		QueueSize:      1000,
		Workers:        4,
		RetryOnFailure: true,
	}
}

// BatchProcessor batches multiple LLM requests for efficient processing.
type BatchProcessor struct {
	config  BatchConfig
	handler BatchHandler
	queue   chan *pendingRequest
	closed  atomic.Bool
	wg      sync.WaitGroup

	// Metrics
	submitted atomic.Int64
	batched   atomic.Int64
	completed atomic.Int64
	failed    atomic.Int64
}

type pendingRequest struct {
	request  *Request
	response chan *Response
	ctx      context.Context
}

// NewBatchProcessor creates a new batch processor.
func NewBatchProcessor(config BatchConfig, handler BatchHandler) *BatchProcessor {
	bp := &BatchProcessor{
		config:  config,
		handler: handler,
		queue:   make(chan *pendingRequest, config.QueueSize),
	}

	// Start workers
	for i := 0; i < config.Workers; i++ {
		bp.wg.Add(1)
		go bp.worker()
	}

	return bp
}

// Submit submits a request and returns a channel for the response.
func (bp *BatchProcessor) Submit(ctx context.Context, req *Request) <-chan *Response {
	respCh := make(chan *Response, 1)

	if bp.closed.Load() {
		respCh <- &Response{ID: req.ID, Error: ErrBatchClosed}
		close(respCh)
		return respCh
	}

	bp.submitted.Add(1)

	pending := &pendingRequest{
		request:  req,
		response: respCh,
		ctx:      ctx,
	}

	select {
	case bp.queue <- pending:
	case <-ctx.Done():
		respCh <- &Response{ID: req.ID, Error: ctx.Err()}
		close(respCh)
	default:
		respCh <- &Response{ID: req.ID, Error: ErrBatchFull}
		close(respCh)
	}

	return respCh
}

// SubmitSync submits a request and waits for the response.
func (bp *BatchProcessor) SubmitSync(ctx context.Context, req *Request) (*Response, error) {
	respCh := bp.Submit(ctx, req)

	select {
	case resp := <-respCh:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp, nil
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (bp *BatchProcessor) worker() {
	defer bp.wg.Done()

	batch := make([]*pendingRequest, 0, bp.config.MaxBatchSize)
	timer := time.NewTimer(bp.config.MaxWaitTime)
	defer timer.Stop()

	for {
		select {
		case pending, ok := <-bp.queue:
			if !ok {
				// Process remaining batch
				if len(batch) > 0 {
					bp.processBatch(batch)
				}
				return
			}

			batch = append(batch, pending)

			if len(batch) >= bp.config.MaxBatchSize {
				bp.processBatch(batch)
				batch = batch[:0]
				timer.Reset(bp.config.MaxWaitTime)
			}

		case <-timer.C:
			if len(batch) > 0 {
				bp.processBatch(batch)
				batch = batch[:0]
			}
			timer.Reset(bp.config.MaxWaitTime)
		}
	}
}

func (bp *BatchProcessor) processBatch(batch []*pendingRequest) {
	if len(batch) == 0 {
		return
	}

	bp.batched.Add(1)

	// Build request slice
	requests := make([]*Request, len(batch))
	for i, p := range batch {
		requests[i] = p.request
	}

	// Use first request's context (or create combined context)
	ctx := batch[0].ctx

	// Process batch
	responses := bp.handler(ctx, requests)

	// Distribute responses
	responseMap := make(map[string]*Response)
	for _, resp := range responses {
		responseMap[resp.ID] = resp
	}

	for _, pending := range batch {
		resp, ok := responseMap[pending.request.ID]
		if !ok {
			resp = &Response{
				ID:    pending.request.ID,
				Error: errors.New("no response for request"),
			}
			bp.failed.Add(1)
		} else if resp.Error != nil {
			bp.failed.Add(1)
		} else {
			bp.completed.Add(1)
		}

		select {
		case pending.response <- resp:
		default:
		}
		close(pending.response)
	}
}

// Close closes the batch processor.
func (bp *BatchProcessor) Close() {
	if bp.closed.Swap(true) {
		return
	}
	close(bp.queue)
	bp.wg.Wait()
}

// Stats returns processor statistics.
func (bp *BatchProcessor) Stats() BatchStats {
	return BatchStats{
		Submitted: bp.submitted.Load(),
		Batched:   bp.batched.Load(),
		Completed: bp.completed.Load(),
		Failed:    bp.failed.Load(),
		Queued:    len(bp.queue),
	}
}

// BatchStats contains processor statistics.
type BatchStats struct {
	Submitted int64 `json:"submitted"`
	Batched   int64 `json:"batched"`
	Completed int64 `json:"completed"`
	Failed    int64 `json:"failed"`
	Queued    int   `json:"queued"`
}

// BatchEfficiency returns the average batch size.
func (s BatchStats) BatchEfficiency() float64 {
	if s.Batched == 0 {
		return 0
	}
	return float64(s.Completed+s.Failed) / float64(s.Batched)
}
