package runtimepolicy

import (
	"context"
	"sync"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

// InMemoryUsageRecorder is a generic reference recorder that stores usage records in memory.
type InMemoryUsageRecorder struct {
	mu      sync.Mutex
	records []*router.ChannelUsageRecord
}

var _ router.UsageRecorder = (*InMemoryUsageRecorder)(nil)

// RecordUsage implements router.UsageRecorder.
func (r *InMemoryUsageRecorder) RecordUsage(_ context.Context, usage *router.ChannelUsageRecord) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = append(r.records, cloneUsageRecord(usage))
	return nil
}

// Snapshot returns cloned usage records in append order.
func (r *InMemoryUsageRecorder) Snapshot() []*router.ChannelUsageRecord {
	r.mu.Lock()
	defer r.mu.Unlock()
	out := make([]*router.ChannelUsageRecord, 0, len(r.records))
	for _, record := range r.records {
		out = append(out, cloneUsageRecord(record))
	}
	return out
}

// Reset clears all in-memory records.
func (r *InMemoryUsageRecorder) Reset() {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.records = nil
}
