package memory

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestInMemoryObservationStore_SaveAndLoad(t *testing.T) {
	store := NewInMemoryObservationStore()
	ctx := context.Background()

	obs1 := Observation{ID: "1", AgentID: "a1", Content: "first", CreatedAt: time.Now()}
	obs2 := Observation{ID: "2", AgentID: "a1", Content: "second", CreatedAt: time.Now()}
	obs3 := Observation{ID: "3", AgentID: "a2", Content: "other agent", CreatedAt: time.Now()}

	require.NoError(t, store.Save(ctx, obs1))
	require.NoError(t, store.Save(ctx, obs2))
	require.NoError(t, store.Save(ctx, obs3))

	results, err := store.LoadRecent(ctx, "a1", 10)
	require.NoError(t, err)
	assert.Len(t, results, 2)
	assert.Equal(t, "second", results[0].Content, "most recent first")
	assert.Equal(t, "first", results[1].Content)

	results, err = store.LoadRecent(ctx, "a1", 1)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "second", results[0].Content)

	results, err = store.LoadRecent(ctx, "a2", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "other agent", results[0].Content)
}

func TestInMemoryObservationStore_LoadByDateRange(t *testing.T) {
	store := NewInMemoryObservationStore()
	ctx := context.Background()

	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	obs1 := Observation{ID: "1", AgentID: "a1", Content: "day1", CreatedAt: base}
	obs2 := Observation{ID: "2", AgentID: "a1", Content: "day2", CreatedAt: base.Add(24 * time.Hour)}
	obs3 := Observation{ID: "3", AgentID: "a1", Content: "day3", CreatedAt: base.Add(48 * time.Hour)}

	require.NoError(t, store.Save(ctx, obs1))
	require.NoError(t, store.Save(ctx, obs2))
	require.NoError(t, store.Save(ctx, obs3))

	results, err := store.LoadByDateRange(ctx, "a1", base, base.Add(24*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 2)

	results, err = store.LoadByDateRange(ctx, "a1", base.Add(48*time.Hour), base.Add(72*time.Hour))
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "day3", results[0].Content)

	results, err = store.LoadByDateRange(ctx, "nonexistent", base, base.Add(72*time.Hour))
	require.NoError(t, err)
	assert.Empty(t, results)
}

func TestObserver_Observe(t *testing.T) {
	mockComplete := func(_ context.Context, _, _ string) (string, error) {
		return "  User decided to use Go for the backend.  ", nil
	}

	obs := NewObserver(ObserverConfig{
		MaxMessagesPerBatch:       50,
		MinMessagesForObservation: 2,
		ObservationInterval:       time.Minute,
	}, mockComplete, nil)

	messages := []types.Message{
		{Role: "user", Content: "Let's use Go for the backend"},
		{Role: "assistant", Content: "Great choice! Go is excellent for backends."},
		{Role: "user", Content: "Agreed, let's proceed."},
	}

	result, err := obs.Observe(context.Background(), "agent-1", messages)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "agent-1", result.AgentID)
	assert.Equal(t, "User decided to use Go for the backend.", result.Content, "content should be trimmed")
	assert.NotEmpty(t, result.ID)
	assert.NotEmpty(t, result.Date)
}

func TestObserver_SkipSmallBatch(t *testing.T) {
	called := false
	mockComplete := func(_ context.Context, _, _ string) (string, error) {
		called = true
		return "", nil
	}

	obs := NewObserver(DefaultObserverConfig(), mockComplete, nil)

	messages := []types.Message{
		{Role: "user", Content: "hi"},
	}

	result, err := obs.Observe(context.Background(), "agent-1", messages)
	require.NoError(t, err)
	assert.Nil(t, result)
	assert.False(t, called, "completion should not be called for small batches")
}

func TestReflector_Reflect(t *testing.T) {
	mockComplete := func(_ context.Context, _, _ string) (string, error) {
		return "  Refined: user prefers Go and confirmed deployment on Kubernetes.  ", nil
	}

	r := NewReflector(mockComplete, nil)

	existing := []Observation{
		{Date: "2025-06-01", Content: "User mentioned interest in Go."},
	}
	draft := &Observation{
		ID:      "obs-1",
		AgentID: "a1",
		Date:    "2025-06-02",
		Content: "User confirmed Go for backend and discussed Kubernetes.",
	}

	result, err := r.Reflect(context.Background(), existing, draft)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Refined: user prefers Go and confirmed deployment on Kubernetes.", result.Content)
	assert.Equal(t, "obs-1", result.ID, "ID preserved")
}

func TestReflector_FallbackOnError(t *testing.T) {
	mockComplete := func(_ context.Context, _, _ string) (string, error) {
		return "", errors.New("LLM unavailable")
	}

	r := NewReflector(mockComplete, nil)

	draft := &Observation{
		ID:      "obs-2",
		AgentID: "a1",
		Date:    "2025-06-02",
		Content: "Original draft content.",
	}

	result, err := r.Reflect(context.Background(), nil, draft)
	require.NoError(t, err)
	require.NotNil(t, result)
	assert.Equal(t, "Original draft content.", result.Content, "should fall back to draft")
}

func TestReflector_NilDraft(t *testing.T) {
	r := NewReflector(nil, nil)
	result, err := r.Reflect(context.Background(), nil, nil)
	require.NoError(t, err)
	assert.Nil(t, result)
}
