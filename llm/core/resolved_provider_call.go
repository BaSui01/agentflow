package core

import (
	"context"
	"sync"
)

// ResolvedProviderCall captures the concrete upstream target chosen for a single request.
type ResolvedProviderCall struct {
	Provider string `json:"provider,omitempty"`
	Model    string `json:"model,omitempty"`
	BaseURL  string `json:"base_url,omitempty"`
}

type resolvedProviderCallRecorderKey struct{}

// ResolvedProviderCallRecorder stores the resolved upstream target selected during a request.
type ResolvedProviderCallRecorder struct {
	mu   sync.RWMutex
	call ResolvedProviderCall
	ok   bool
}

// WithResolvedProviderCallRecorder attaches a recorder to ctx for downstream providers to report into.
func WithResolvedProviderCallRecorder(ctx context.Context) (context.Context, *ResolvedProviderCallRecorder) {
	if ctx == nil {
		ctx = context.Background()
	}
	if recorder, ok := ctx.Value(resolvedProviderCallRecorderKey{}).(*ResolvedProviderCallRecorder); ok && recorder != nil {
		return ctx, recorder
	}
	recorder := &ResolvedProviderCallRecorder{}
	return context.WithValue(ctx, resolvedProviderCallRecorderKey{}, recorder), recorder
}

// RecordResolvedProviderCall reports the concrete provider/model/baseURL chosen for the current request.
func RecordResolvedProviderCall(ctx context.Context, call ResolvedProviderCall) {
	if ctx == nil {
		return
	}
	recorder, ok := ctx.Value(resolvedProviderCallRecorderKey{}).(*ResolvedProviderCallRecorder)
	if !ok || recorder == nil {
		return
	}
	recorder.Store(call)
}

// Store saves the resolved provider call.
func (r *ResolvedProviderCallRecorder) Store(call ResolvedProviderCall) {
	if r == nil {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.call = call
	r.ok = true
}

// Load returns the last resolved provider call stored in the recorder.
func (r *ResolvedProviderCallRecorder) Load() (ResolvedProviderCall, bool) {
	if r == nil {
		return ResolvedProviderCall{}, false
	}
	r.mu.RLock()
	defer r.mu.RUnlock()
	if !r.ok {
		return ResolvedProviderCall{}, false
	}
	return r.call, true
}
