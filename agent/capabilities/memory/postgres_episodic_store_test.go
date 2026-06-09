package memory

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BaSui01/agentflow/types"
	"github.com/stretchr/testify/require"
)

type mockEpisodicRow struct {
	values []any
}

func (r *mockEpisodicRow) Scan(dest ...any) error {
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
			case string:
				*d = &sv
			case *string:
				*d = sv
			}
		case *time.Time:
			*d = v.(time.Time)
		case *int64:
			*d = v.(int64)
		default:
			return fmt.Errorf("unsupported scan target type at index %d", i)
		}
	}
	return nil
}

type mockEpisodicRows struct {
	data [][]any
	idx  int
}

func (r *mockEpisodicRows) Next() bool {
	if r.idx < len(r.data) {
		r.idx++
		return true
	}
	return false
}

func (r *mockEpisodicRows) Scan(dest ...any) error {
	return (&mockEpisodicRow{values: r.data[r.idx-1]}).Scan(dest...)
}

func (r *mockEpisodicRows) Close() error { return nil }

type mockEpisodicDBClient struct {
	mu   sync.Mutex
	data []types.EpisodicEvent
}

func (c *mockEpisodicDBClient) Exec(_ context.Context, _ string, args ...any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(args) == 0 {
		return nil
	}
	if len(args) < 7 {
		return fmt.Errorf("unexpected exec args: %d", len(args))
	}
	event := types.EpisodicEvent{
		ID:        args[0].(string),
		AgentID:   args[1].(string),
		Type:      args[2].(string),
		Content:   args[3].(string),
		Timestamp: args[5].(time.Time),
		Duration:  time.Duration(args[6].(int64)),
	}
	for i, existing := range c.data {
		if existing.ID == event.ID {
			c.data[i] = event
			return nil
		}
	}
	c.data = append(c.data, event)
	return nil
}

func (c *mockEpisodicDBClient) Query(_ context.Context, query string, args ...any) (EpisodicDBRows, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	var limit int
	filtered := make([]types.EpisodicEvent, 0, len(c.data))
	for _, ev := range c.data {
		if len(args) > 0 {
			if agentID, ok := args[0].(string); ok && agentID != "" && ev.AgentID != agentID {
				continue
			}
		}
		filtered = append(filtered, ev)
	}
	if len(args) > 0 {
		if last, ok := args[len(args)-1].(int); ok {
			limit = last
		}
	}

	desc := strings.Contains(query, "DESC")
	for i := 0; i < len(filtered); i++ {
		for j := i + 1; j < len(filtered); j++ {
			swap := filtered[j].Timestamp.Before(filtered[i].Timestamp)
			if desc {
				swap = filtered[j].Timestamp.After(filtered[i].Timestamp)
			}
			if swap {
				filtered[i], filtered[j] = filtered[j], filtered[i]
			}
		}
	}
	if limit > 0 && len(filtered) > limit {
		filtered = filtered[:limit]
	}

	rows := make([][]any, 0, len(filtered))
	for _, ev := range filtered {
		rows = append(rows, []any{ev.ID, ev.AgentID, ev.Type, ev.Content, nil, ev.Timestamp, int64(ev.Duration)})
	}
	return &mockEpisodicRows{data: rows}, nil
}

func TestPostgreSQLEpisodicStore_RecordAndQueryAcrossStoreInstances(t *testing.T) {
	ctx := context.Background()
	client := &mockEpisodicDBClient{}
	store1, err := NewPostgreSQLEpisodicStore(ctx, client)
	require.NoError(t, err)

	base := time.Date(2026, 5, 12, 9, 0, 0, 0, time.UTC)
	require.NoError(t, store1.RecordEvent(ctx, &types.EpisodicEvent{
		ID:        "ep-1",
		AgentID:   "agent-1",
		Type:      "task_started",
		Content:   "started",
		Timestamp: base,
		Duration:  time.Second,
	}))
	require.NoError(t, store1.RecordEvent(ctx, &types.EpisodicEvent{
		ID:        "ep-2",
		AgentID:   "agent-1",
		Type:      "task_completed",
		Content:   "completed",
		Timestamp: base.Add(time.Hour),
		Duration:  2 * time.Second,
	}))

	store2, err := NewPostgreSQLEpisodicStore(ctx, client)
	require.NoError(t, err)
	events, err := store2.QueryEvents(ctx, EpisodicQuery{AgentID: "agent-1", Limit: 10})
	require.NoError(t, err)
	require.Len(t, events, 2)
	require.Equal(t, "ep-2", events[0].ID)
	require.Equal(t, "ep-1", events[1].ID)

	timeline, err := store2.GetTimeline(ctx, "agent-1", base.Add(-time.Minute), base.Add(2*time.Hour))
	require.NoError(t, err)
	require.Len(t, timeline, 2)
	require.Equal(t, "ep-1", timeline[0].ID)
	require.Equal(t, "ep-2", timeline[1].ID)
}
