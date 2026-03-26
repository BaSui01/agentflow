package runtimepolicy

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	router "github.com/BaSui01/agentflow/llm/runtime/router"
)

type captureUsageRecorder struct {
	records []*router.ChannelUsageRecord
	err     error
}

func (r *captureUsageRecorder) RecordUsage(_ context.Context, usage *router.ChannelUsageRecord) error {
	r.records = append(r.records, cloneUsageRecord(usage))
	return r.err
}

type captureCooldownController struct {
	allows  []*router.ChannelSelection
	records []*router.ChannelUsageRecord
	err     error
}

func (c *captureCooldownController) Allow(_ context.Context, _ *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	c.allows = append(c.allows, cloneSelection(selection))
	return c.err
}

func (c *captureCooldownController) RecordResult(_ context.Context, usage *router.ChannelUsageRecord) error {
	c.records = append(c.records, cloneUsageRecord(usage))
	return c.err
}

type captureQuotaPolicy struct {
	allows  []*router.ChannelSelection
	records []*router.ChannelUsageRecord
	err     error
}

func (p *captureQuotaPolicy) Allow(_ context.Context, _ *router.ChannelRouteRequest, selection *router.ChannelSelection) error {
	p.allows = append(p.allows, cloneSelection(selection))
	return p.err
}

func (p *captureQuotaPolicy) RecordUsage(_ context.Context, usage *router.ChannelUsageRecord) error {
	p.records = append(p.records, cloneUsageRecord(usage))
	return p.err
}

func TestMultiUsageRecorder_JoinsErrorsAndClonesInput(t *testing.T) {
	t.Parallel()

	first := &captureUsageRecorder{}
	second := &captureUsageRecorder{err: errors.New("recorder-b")}
	multi := MultiUsageRecorder{Recorders: []router.UsageRecorder{first, second}}

	record := &router.ChannelUsageRecord{
		KeyID:    "key-a",
		Metadata: map[string]string{"scope": "a"},
		Usage:    &router.ChatUsage{TotalTokens: 9},
	}
	err := multi.RecordUsage(context.Background(), record)
	require.Error(t, err)
	require.ErrorContains(t, err, "recorder-b")

	record.Metadata["scope"] = "mutated"
	record.Usage.TotalTokens = 99

	require.Len(t, first.records, 1)
	require.Equal(t, "a", first.records[0].Metadata["scope"])
	require.Equal(t, 9, first.records[0].Usage.TotalTokens)
	require.Len(t, second.records, 1)
	require.Equal(t, "a", second.records[0].Metadata["scope"])
}

func TestMultiCooldownController_JoinsErrorsAndClonesInput(t *testing.T) {
	t.Parallel()

	first := &captureCooldownController{}
	second := &captureCooldownController{err: errors.New("cooldown-b")}
	multi := MultiCooldownController{Controllers: []router.CooldownController{first, second}}

	request := &router.ChannelRouteRequest{RequestedModel: "gpt-4o", Metadata: map[string]string{"region": "us"}}
	selection := &router.ChannelSelection{ChannelID: "channel-a", KeyID: "key-a", Metadata: map[string]string{"route": "primary"}}
	err := multi.Allow(context.Background(), request, selection)
	require.Error(t, err)
	require.ErrorContains(t, err, "cooldown-b")

	selection.Metadata["route"] = "mutated"
	require.Len(t, first.allows, 1)
	require.Equal(t, "primary", first.allows[0].Metadata["route"])
	require.Len(t, second.allows, 1)

	record := &router.ChannelUsageRecord{ChannelID: "channel-a", KeyID: "key-a"}
	err = multi.RecordResult(context.Background(), record)
	require.Error(t, err)
	require.ErrorContains(t, err, "cooldown-b")
	require.Len(t, first.records, 1)
	require.Len(t, second.records, 1)
}

func TestMultiQuotaPolicy_JoinsErrorsAndClonesInput(t *testing.T) {
	t.Parallel()

	first := &captureQuotaPolicy{}
	second := &captureQuotaPolicy{err: errors.New("quota-b")}
	multi := MultiQuotaPolicy{Policies: []router.QuotaPolicy{first, second}}

	selection := &router.ChannelSelection{ChannelID: "channel-a", KeyID: "key-a", Metadata: map[string]string{"scope": "route-a"}}
	err := multi.Allow(context.Background(), &router.ChannelRouteRequest{}, selection)
	require.Error(t, err)
	require.ErrorContains(t, err, "quota-b")

	selection.Metadata["scope"] = "mutated"
	require.Len(t, first.allows, 1)
	require.Equal(t, "route-a", first.allows[0].Metadata["scope"])

	record := &router.ChannelUsageRecord{KeyID: "key-a", Usage: &router.ChatUsage{TotalTokens: 5}}
	err = multi.RecordUsage(context.Background(), record)
	require.Error(t, err)
	require.ErrorContains(t, err, "quota-b")
	require.Len(t, first.records, 1)
	require.Equal(t, 5, first.records[0].Usage.TotalTokens)
}

func TestInMemoryUsageRecorder_SnapshotReturnsClones(t *testing.T) {
	t.Parallel()

	recorder := &InMemoryUsageRecorder{}
	record := &router.ChannelUsageRecord{
		KeyID:    "key-a",
		Metadata: map[string]string{"scope": "alpha"},
		Usage:    &router.ChatUsage{TotalTokens: 7},
	}
	require.NoError(t, recorder.RecordUsage(context.Background(), record))

	record.Metadata["scope"] = "mutated"
	record.Usage.TotalTokens = 99

	snapshot := recorder.Snapshot()
	require.Len(t, snapshot, 1)
	require.Equal(t, "alpha", snapshot[0].Metadata["scope"])
	require.Equal(t, 7, snapshot[0].Usage.TotalTokens)

	snapshot[0].Metadata["scope"] = "changed"
	require.Equal(t, "alpha", recorder.Snapshot()[0].Metadata["scope"])
}

func TestInMemoryCooldownController_RejectsActiveCooldownAndExpires(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	controller := &InMemoryCooldownController{
		Decider: FailureCooldownDecider{KeyTTL: time.Hour, ChannelTTL: 2 * time.Hour},
		Now: func() time.Time {
			return now
		},
	}

	record := &router.ChannelUsageRecord{
		ChannelID: "channel-a",
		KeyID:     "key-a",
		Success:   false,
	}
	require.NoError(t, controller.RecordResult(context.Background(), record))

	err := controller.Allow(context.Background(), nil, &router.ChannelSelection{ChannelID: "channel-a", KeyID: "key-a"})
	require.Error(t, err)
	require.ErrorContains(t, err, "key-a")

	snapshot := controller.Snapshot()
	require.Contains(t, snapshot.KeyCooldowns, "key-a")
	require.Contains(t, snapshot.ChannelCooldowns, "channel-a")

	now = now.Add(3 * time.Hour)
	require.NoError(t, controller.Allow(context.Background(), nil, &router.ChannelSelection{ChannelID: "channel-a", KeyID: "key-a"}))
	snapshot = controller.Snapshot()
	require.NotContains(t, snapshot.KeyCooldowns, "key-a")
	require.NotContains(t, snapshot.ChannelCooldowns, "channel-a")
}

func TestInMemoryQuotaPolicy_EnforcesDailyLimitsAndReleasesConcurrency(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC)
	policy := NewInMemoryQuotaPolicy(InMemoryQuotaPolicyConfig{
		KeyLimits:     QuotaLimits{DailyRequests: 2, DailyTokens: 10, Concurrency: 1},
		ChannelLimits: QuotaLimits{DailyRequests: 3, DailyTokens: 20, Concurrency: 1},
		Now: func() time.Time {
			return now
		},
	})
	selection := &router.ChannelSelection{ChannelID: "channel-a", KeyID: "key-a"}

	require.NoError(t, policy.Allow(context.Background(), nil, selection))
	err := policy.Allow(context.Background(), nil, selection)
	require.Error(t, err)
	require.ErrorContains(t, err, "concurrency")

	require.NoError(t, policy.RecordUsage(context.Background(), &router.ChannelUsageRecord{
		ChannelID: "channel-a",
		KeyID:     "key-a",
		Success:   true,
		Usage:     &router.ChatUsage{TotalTokens: 6},
	}))

	require.NoError(t, policy.Allow(context.Background(), nil, selection))
	require.NoError(t, policy.RecordUsage(context.Background(), &router.ChannelUsageRecord{
		ChannelID: "channel-a",
		KeyID:     "key-a",
		Success:   true,
		Usage:     &router.ChatUsage{TotalTokens: 5},
	}))

	err = policy.Allow(context.Background(), nil, selection)
	require.Error(t, err)
	require.ErrorContains(t, err, "daily request limit")

	snapshot := policy.Snapshot()
	require.Equal(t, int64(2), snapshot.KeyCounters["key-a"].Requests)
	require.Equal(t, int64(11), snapshot.KeyCounters["key-a"].Tokens)
	require.Equal(t, int64(2), snapshot.ChannelCounters["channel-a"].Requests)
	require.Equal(t, int64(11), snapshot.ChannelCounters["channel-a"].Tokens)
	require.Empty(t, snapshot.KeyInflight)
	require.Empty(t, snapshot.ChannelInflight)
}

func TestInMemoryQuotaPolicy_CanSkipFailedAttempts(t *testing.T) {
	t.Parallel()

	policy := NewInMemoryQuotaPolicy(InMemoryQuotaPolicyConfig{
		KeyLimits:           QuotaLimits{DailyRequests: 1},
		CountOnlySuccessful: true,
		Now:                 func() time.Time { return time.Date(2026, 3, 26, 12, 0, 0, 0, time.UTC) },
	})
	selection := &router.ChannelSelection{KeyID: "key-a"}

	require.NoError(t, policy.Allow(context.Background(), nil, selection))
	require.NoError(t, policy.RecordUsage(context.Background(), &router.ChannelUsageRecord{
		KeyID:   "key-a",
		Success: false,
	}))

	require.NoError(t, policy.Allow(context.Background(), nil, selection))
}
