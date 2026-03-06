package memory

import (
	"context"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// --- In-memory mock of agent.PostgreSQLClient ---

type mockRow struct {
	values []any
}

func (r *mockRow) Scan(dest ...any) error {
	if len(dest) != len(r.values) {
		return fmt.Errorf("scan: expected %d cols, got %d", len(r.values), len(dest))
	}
	for i, v := range r.values {
		switch d := dest[i].(type) {
		case *string:
			*d = v.(string)
		case **string:
			switch sv := v.(type) {
			case nil:
				*d = nil
			case *string:
				*d = sv
			case string:
				*d = &sv
			}
		case *time.Time:
			*d = v.(time.Time)
		default:
			return fmt.Errorf("unsupported scan target type at index %d", i)
		}
	}
	return nil
}

type mockRows struct {
	data [][]any
	idx  int
}

func (r *mockRows) Next() bool {
	if r.idx < len(r.data) {
		r.idx++
		return true
	}
	return false
}

func (r *mockRows) Scan(dest ...any) error {
	row := &mockRow{values: r.data[r.idx-1]}
	return row.Scan(dest...)
}

func (r *mockRows) Close() error { return nil }

type observation struct {
	id, agentID, date, content string
	createdAt                  time.Time
	metadata                   *string
}

type mockPGClient struct {
	mu     sync.Mutex
	data   []observation
	execOK bool
}

func newMockPGClient() *mockPGClient {
	return &mockPGClient{execOK: true}
}

func (c *mockPGClient) Exec(_ context.Context, query string, args ...any) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if len(args) == 0 {
		return nil
	}

	if len(args) >= 6 {
		var metaPtr *string
		if args[5] != nil {
			if sp, ok := args[5].(*string); ok {
				metaPtr = sp
			}
		}
		obs := observation{
			id:        args[0].(string),
			agentID:   args[1].(string),
			date:      args[2].(string),
			content:   args[3].(string),
			createdAt: args[4].(time.Time),
			metadata:  metaPtr,
		}
		for i, existing := range c.data {
			if existing.id == obs.id {
				c.data[i] = obs
				return nil
			}
		}
		c.data = append(c.data, obs)
	}
	return nil
}

func (c *mockPGClient) Query(_ context.Context, query string, args ...any) (DBRows, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	agentID := args[0].(string)
	var filtered []observation

	for _, o := range c.data {
		if o.agentID != agentID {
			continue
		}
		filtered = append(filtered, o)
	}

	if len(args) >= 3 {
		start := args[1].(time.Time)
		end := args[2].(time.Time)
		var dateFiltered []observation
		for _, o := range filtered {
			if !o.createdAt.Before(start) && !o.createdAt.After(end) {
				dateFiltered = append(dateFiltered, o)
			}
		}
		filtered = dateFiltered
	} else {
		limit := args[1].(int)
		for i, j := 0, len(filtered)-1; i < j; i, j = i+1, j-1 {
			filtered[i], filtered[j] = filtered[j], filtered[i]
		}
		if len(filtered) > limit {
			filtered = filtered[:limit]
		}
	}

	var rows [][]any
	for _, o := range filtered {
		var meta any = o.metadata
		rows = append(rows, []any{o.id, o.agentID, o.date, o.content, o.createdAt, meta})
	}
	return &mockRows{data: rows}, nil
}

// --- tests ---

func TestPostgreSQLObservationStore_SaveAndLoadRecent(t *testing.T) {
	ctx := context.Background()
	client := newMockPGClient()
	store, err := NewPostgreSQLObservationStore(ctx, client)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	obs1 := Observation{ID: "1", AgentID: "a1", Date: "2025-06-01", Content: "first", CreatedAt: now.Add(-2 * time.Hour)}
	obs2 := Observation{ID: "2", AgentID: "a1", Date: "2025-06-01", Content: "second", CreatedAt: now.Add(-1 * time.Hour)}
	obs3 := Observation{ID: "3", AgentID: "a2", Date: "2025-06-01", Content: "other agent", CreatedAt: now}

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
}

func TestPostgreSQLObservationStore_LoadByDateRange(t *testing.T) {
	ctx := context.Background()
	client := newMockPGClient()
	store, err := NewPostgreSQLObservationStore(ctx, client)
	require.NoError(t, err)

	base := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)
	obs1 := Observation{ID: "1", AgentID: "a1", Date: "2025-06-01", Content: "day1", CreatedAt: base}
	obs2 := Observation{ID: "2", AgentID: "a1", Date: "2025-06-02", Content: "day2", CreatedAt: base.Add(24 * time.Hour)}
	obs3 := Observation{ID: "3", AgentID: "a1", Date: "2025-06-03", Content: "day3", CreatedAt: base.Add(48 * time.Hour)}

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
}

func TestPostgreSQLObservationStore_Metadata(t *testing.T) {
	ctx := context.Background()
	client := newMockPGClient()
	store, err := NewPostgreSQLObservationStore(ctx, client)
	require.NoError(t, err)

	obs := Observation{
		ID:        "m1",
		AgentID:   "a1",
		Date:      "2025-06-01",
		Content:   "with metadata",
		CreatedAt: time.Now().UTC().Truncate(time.Second),
		Metadata: map[string]any{
			"source":  "conversation",
			"turn_id": float64(42),
		},
	}
	require.NoError(t, store.Save(ctx, obs))

	results, err := store.LoadRecent(ctx, "a1", 1)
	require.NoError(t, err)
	require.Len(t, results, 1)
	assert.Equal(t, "conversation", results[0].Metadata["source"])
	assert.Equal(t, float64(42), results[0].Metadata["turn_id"])
}

func TestPostgreSQLObservationStore_Upsert(t *testing.T) {
	ctx := context.Background()
	client := newMockPGClient()
	store, err := NewPostgreSQLObservationStore(ctx, client)
	require.NoError(t, err)

	now := time.Now().UTC().Truncate(time.Second)
	obs := Observation{ID: "u1", AgentID: "a1", Date: "2025-06-01", Content: "original", CreatedAt: now}
	require.NoError(t, store.Save(ctx, obs))

	obs.Content = "updated"
	require.NoError(t, store.Save(ctx, obs))

	results, err := store.LoadRecent(ctx, "a1", 10)
	require.NoError(t, err)
	assert.Len(t, results, 1)
	assert.Equal(t, "updated", results[0].Content)
}

func TestPostgreSQLObservationStore_NilDB(t *testing.T) {
	_, err := NewPostgreSQLObservationStore(context.Background(), nil)
	assert.Error(t, err)
}

func TestPostgreSQLObservationStore_EmptyResults(t *testing.T) {
	ctx := context.Background()
	client := newMockPGClient()
	store, err := NewPostgreSQLObservationStore(ctx, client)
	require.NoError(t, err)

	results, err := store.LoadRecent(ctx, "nobody", 10)
	require.NoError(t, err)
	assert.Empty(t, results)
}
