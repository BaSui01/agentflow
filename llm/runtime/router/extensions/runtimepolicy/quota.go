package runtimepolicy

import (
	"context"
	"fmt"
	"sync"
	"time"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
	"github.com/BaSui01/agentflow/types"
)

// QuotaLimits describes generic per-key or per-channel limits.
type QuotaLimits struct {
	DailyRequests int64
	DailyTokens   int64
	Concurrency   int
}

// InMemoryQuotaPolicyConfig controls the reference quota policy behavior.
type InMemoryQuotaPolicyConfig struct {
	KeyLimits           QuotaLimits
	ChannelLimits       QuotaLimits
	CountOnlySuccessful bool
	Location            *time.Location
	Now                 func() time.Time
}

// QuotaCounterSnapshot captures accumulated usage for one scope and day.
type QuotaCounterSnapshot struct {
	Requests int64
	Tokens   int64
}

// QuotaSnapshot returns a cloned view of the in-memory quota state.
type QuotaSnapshot struct {
	Day             string
	KeyCounters     map[string]QuotaCounterSnapshot
	ChannelCounters map[string]QuotaCounterSnapshot
	KeyInflight     map[string]int
	ChannelInflight map[string]int
}

type quotaCounter struct {
	Requests int64
	Tokens   int64
}

type quotaCounterKey struct {
	scope quotaScope
	id    string
	day   string
}

type quotaScope string

const (
	quotaScopeKey     quotaScope = "key"
	quotaScopeChannel quotaScope = "channel"
)

// InMemoryQuotaPolicy is a generic reference quota policy with daily and concurrency limits.
type InMemoryQuotaPolicy struct {
	config InMemoryQuotaPolicyConfig

	mu       sync.Mutex
	counters map[quotaCounterKey]quotaCounter
	inflight map[quotaCounterKey]int
}

var _ router.QuotaPolicy = (*InMemoryQuotaPolicy)(nil)

// NewInMemoryQuotaPolicy creates the reference quota policy.
func NewInMemoryQuotaPolicy(cfg InMemoryQuotaPolicyConfig) *InMemoryQuotaPolicy {
	return &InMemoryQuotaPolicy{
		config:   cfg,
		counters: map[quotaCounterKey]quotaCounter{},
		inflight: map[quotaCounterKey]int{},
	}
}

// Allow implements router.QuotaPolicy.
func (p *InMemoryQuotaPolicy) Allow(_ context.Context, _ *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	if selection == nil {
		return nil
	}

	now := p.now()
	day := p.dayKey(now)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneCountersLocked(day)

	if err := p.checkLimitLocked(quotaScopeKey, selection.KeyID, p.config.KeyLimits, day); err != nil {
		return err
	}
	if err := p.checkLimitLocked(quotaScopeChannel, selection.ChannelID, p.config.ChannelLimits, day); err != nil {
		return err
	}

	p.incrementInflightLocked(quotaScopeKey, selection.KeyID, day, 1)
	p.incrementInflightLocked(quotaScopeChannel, selection.ChannelID, day, 1)
	return nil
}

// RecordUsage implements router.QuotaPolicy.
func (p *InMemoryQuotaPolicy) RecordUsage(_ context.Context, usage *router.ChannelUsageRecord) error {
	if usage == nil {
		return nil
	}

	now := p.now()
	day := p.dayKey(now)
	tokens := int64(0)
	if usage.Usage != nil {
		tokens = int64(usage.Usage.TotalTokens)
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneCountersLocked(day)

	p.incrementInflightLocked(quotaScopeKey, usage.KeyID, day, -1)
	p.incrementInflightLocked(quotaScopeChannel, usage.ChannelID, day, -1)

	if p.config.CountOnlySuccessful && !usage.Success {
		return nil
	}

	p.incrementCounterLocked(quotaScopeKey, usage.KeyID, day, 1, tokens)
	p.incrementCounterLocked(quotaScopeChannel, usage.ChannelID, day, 1, tokens)
	return nil
}

// Snapshot returns cloned counters and inflight counts for the current day.
func (p *InMemoryQuotaPolicy) Snapshot() QuotaSnapshot {
	now := p.now()
	day := p.dayKey(now)

	p.mu.Lock()
	defer p.mu.Unlock()
	p.pruneCountersLocked(day)

	snapshot := QuotaSnapshot{
		Day:             day,
		KeyCounters:     map[string]QuotaCounterSnapshot{},
		ChannelCounters: map[string]QuotaCounterSnapshot{},
		KeyInflight:     map[string]int{},
		ChannelInflight: map[string]int{},
	}
	for key, counter := range p.counters {
		if key.day != day {
			continue
		}
		switch key.scope {
		case quotaScopeKey:
			snapshot.KeyCounters[key.id] = QuotaCounterSnapshot{Requests: counter.Requests, Tokens: counter.Tokens}
		case quotaScopeChannel:
			snapshot.ChannelCounters[key.id] = QuotaCounterSnapshot{Requests: counter.Requests, Tokens: counter.Tokens}
		}
	}
	for key, inflight := range p.inflight {
		if key.day != day || inflight == 0 {
			continue
		}
		switch key.scope {
		case quotaScopeKey:
			snapshot.KeyInflight[key.id] = inflight
		case quotaScopeChannel:
			snapshot.ChannelInflight[key.id] = inflight
		}
	}
	if len(snapshot.KeyCounters) == 0 {
		snapshot.KeyCounters = nil
	}
	if len(snapshot.ChannelCounters) == 0 {
		snapshot.ChannelCounters = nil
	}
	if len(snapshot.KeyInflight) == 0 {
		snapshot.KeyInflight = nil
	}
	if len(snapshot.ChannelInflight) == 0 {
		snapshot.ChannelInflight = nil
	}
	return snapshot
}

func (p *InMemoryQuotaPolicy) checkLimitLocked(scope quotaScope, id string, limits QuotaLimits, day string) error {
	if id == "" {
		return nil
	}
	key := quotaCounterKey{scope: scope, id: id, day: day}
	counter := p.counters[key]
	inflight := p.inflight[key]

	if limits.Concurrency > 0 && inflight >= limits.Concurrency {
		return types.NewRateLimitError(fmt.Sprintf("channel route %s %s exceeded concurrency limit", scope, id))
	}
	if limits.DailyRequests > 0 && counter.Requests >= limits.DailyRequests {
		return types.NewRateLimitError(fmt.Sprintf("channel route %s %s exceeded daily request limit", scope, id))
	}
	if limits.DailyTokens > 0 && counter.Tokens >= limits.DailyTokens {
		return types.NewRateLimitError(fmt.Sprintf("channel route %s %s exceeded daily token limit", scope, id))
	}
	return nil
}

func (p *InMemoryQuotaPolicy) incrementCounterLocked(scope quotaScope, id string, day string, requests int64, tokens int64) {
	if id == "" {
		return
	}
	key := quotaCounterKey{scope: scope, id: id, day: day}
	counter := p.counters[key]
	counter.Requests += requests
	counter.Tokens += tokens
	p.counters[key] = counter
}

func (p *InMemoryQuotaPolicy) incrementInflightLocked(scope quotaScope, id string, day string, delta int) {
	if id == "" || delta == 0 {
		return
	}
	key := quotaCounterKey{scope: scope, id: id, day: day}
	next := p.inflight[key] + delta
	if next <= 0 {
		delete(p.inflight, key)
		return
	}
	p.inflight[key] = next
}

func (p *InMemoryQuotaPolicy) pruneCountersLocked(day string) {
	for key := range p.counters {
		if key.day != day {
			delete(p.counters, key)
		}
	}
	for key := range p.inflight {
		if key.day != day {
			delete(p.inflight, key)
		}
	}
}

func (p *InMemoryQuotaPolicy) now() time.Time {
	if p != nil && p.config.Now != nil {
		return p.config.Now()
	}
	return time.Now()
}

func (p *InMemoryQuotaPolicy) dayKey(now time.Time) string {
	location := p.config.Location
	if location == nil {
		location = time.UTC
	}
	return now.In(location).Format("2006-01-02")
}
