package checkpoint

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"testing"
	"time"

	agentpkg "github.com/BaSui01/agentflow/agent"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

type mockPostgresClient struct {
	records map[string]postgresRecord
	threads map[string][]string
}

type postgresRecord struct {
	id        string
	threadID  string
	version   int
	state     agentpkg.State
	data      []byte
	createdAt time.Time
}

func newMockPostgresClient() *mockPostgresClient {
	return &mockPostgresClient{
		records: make(map[string]postgresRecord),
		threads: make(map[string][]string),
	}
}

func (c *mockPostgresClient) Exec(_ context.Context, query string, args ...any) error {
	switch {
	case strings.Contains(query, "INSERT INTO agent_checkpoints"):
		record := postgresRecord{
			id:        args[0].(string),
			threadID:  args[1].(string),
			version:   args[3].(int),
			state:     agentpkg.State(fmt.Sprint(args[4])),
			data:      append([]byte(nil), args[5].([]byte)...),
			createdAt: args[6].(time.Time),
		}
		c.records[record.id] = record
		if !containsString(c.threads[record.threadID], record.id) {
			c.threads[record.threadID] = append(c.threads[record.threadID], record.id)
		}
		return nil
	case strings.Contains(query, "DELETE FROM agent_checkpoints WHERE id ="):
		id := args[0].(string)
		record, ok := c.records[id]
		if !ok {
			return nil
		}
		delete(c.records, id)
		c.threads[record.threadID] = removeString(c.threads[record.threadID], id)
		return nil
	case strings.Contains(query, "DELETE FROM agent_checkpoints WHERE thread_id ="):
		threadID := args[0].(string)
		for _, id := range c.threads[threadID] {
			delete(c.records, id)
		}
		delete(c.threads, threadID)
		return nil
	default:
		return nil
	}
}

func (c *mockPostgresClient) QueryRow(_ context.Context, query string, args ...any) Row {
	switch {
	case strings.Contains(query, "WHERE id = $1"):
		id := args[0].(string)
		record, ok := c.records[id]
		if !ok {
			return mockPostgresRow{err: fmt.Errorf("not found")}
		}
		return mockPostgresRow{values: []any{append([]byte(nil), record.data...)}}
	case strings.Contains(query, "ORDER BY created_at DESC"):
		threadID := args[0].(string)
		record, ok := c.latestRecord(threadID)
		if !ok {
			return mockPostgresRow{err: fmt.Errorf("not found")}
		}
		return mockPostgresRow{values: []any{append([]byte(nil), record.data...)}}
	case strings.Contains(query, "version = $2"):
		threadID := args[0].(string)
		version := args[1].(int)
		record, ok := c.versionRecord(threadID, version)
		if !ok {
			return mockPostgresRow{err: fmt.Errorf("not found")}
		}
		return mockPostgresRow{values: []any{append([]byte(nil), record.data...)}}
	default:
		return mockPostgresRow{err: fmt.Errorf("unsupported query")}
	}
}

func (c *mockPostgresClient) Query(_ context.Context, query string, args ...any) (Rows, error) {
	switch {
	case strings.Contains(query, "SELECT data FROM agent_checkpoints"):
		threadID := args[0].(string)
		limit := args[1].(int)
		records := c.threadRecords(threadID)
		sort.Slice(records, func(i, j int) bool {
			return records[i].createdAt.After(records[j].createdAt)
		})
		if limit < len(records) {
			records = records[:limit]
		}
		rows := make([][]any, 0, len(records))
		for _, record := range records {
			rows = append(rows, []any{append([]byte(nil), record.data...)})
		}
		return &mockPostgresRows{rows: rows}, nil
	case strings.Contains(query, "SELECT id, version, created_at, state FROM agent_checkpoints"):
		threadID := args[0].(string)
		records := c.threadRecords(threadID)
		sort.Slice(records, func(i, j int) bool {
			return records[i].version < records[j].version
		})
		rows := make([][]any, 0, len(records))
		for _, record := range records {
			rows = append(rows, []any{record.id, record.version, record.createdAt, string(record.state)})
		}
		return &mockPostgresRows{rows: rows}, nil
	default:
		return nil, fmt.Errorf("unsupported query")
	}
}

func (c *mockPostgresClient) latestRecord(threadID string) (postgresRecord, bool) {
	records := c.threadRecords(threadID)
	if len(records) == 0 {
		return postgresRecord{}, false
	}
	sort.Slice(records, func(i, j int) bool {
		return records[i].createdAt.After(records[j].createdAt)
	})
	return records[0], true
}

func (c *mockPostgresClient) versionRecord(threadID string, version int) (postgresRecord, bool) {
	for _, record := range c.threadRecords(threadID) {
		if record.version == version {
			return record, true
		}
	}
	return postgresRecord{}, false
}

func (c *mockPostgresClient) threadRecords(threadID string) []postgresRecord {
	ids := c.threads[threadID]
	records := make([]postgresRecord, 0, len(ids))
	for _, id := range ids {
		if record, ok := c.records[id]; ok {
			records = append(records, record)
		}
	}
	return records
}

type mockPostgresRow struct {
	values []any
	err    error
}

func (r mockPostgresRow) Scan(dest ...any) error {
	if r.err != nil {
		return r.err
	}
	return scanValues(dest, r.values)
}

type mockPostgresRows struct {
	rows [][]any
	idx  int
}

func (r *mockPostgresRows) Next() bool {
	if r.idx >= len(r.rows) {
		return false
	}
	r.idx++
	return true
}

func (r *mockPostgresRows) Scan(dest ...any) error {
	return scanValues(dest, r.rows[r.idx-1])
}

func (r *mockPostgresRows) Close() error { return nil }

func scanValues(dest []any, values []any) error {
	if len(dest) != len(values) {
		return fmt.Errorf("scan mismatch: got %d destinations for %d values", len(dest), len(values))
	}
	for i, value := range values {
		switch target := dest[i].(type) {
		case *[]byte:
			*target = append([]byte(nil), value.([]byte)...)
		case *string:
			*target = value.(string)
		case *int:
			*target = value.(int)
		case *time.Time:
			*target = value.(time.Time)
		default:
			return fmt.Errorf("unsupported scan destination %T", target)
		}
	}
	return nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func removeString(values []string, target string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if value != target {
			result = append(result, value)
		}
	}
	return result
}

func TestPostgreSQLCheckpointStore_SaveLoadAndListVersions(t *testing.T) {
	client := newMockPostgresClient()
	store := NewPostgreSQLCheckpointStore(client, zap.NewNop())

	cp1 := &agentpkg.Checkpoint{ID: "cp-1", ThreadID: "t1", AgentID: "a1", State: agentpkg.StateInit, CreatedAt: time.Now().Add(-time.Minute)}
	cp2 := &agentpkg.Checkpoint{ID: "cp-2", ThreadID: "t1", AgentID: "a1", State: agentpkg.StateReady, CreatedAt: time.Now()}

	require.NoError(t, store.Save(context.Background(), cp1))
	require.NoError(t, store.Save(context.Background(), cp2))

	loaded, err := store.Load(context.Background(), "cp-1")
	require.NoError(t, err)
	assert.Equal(t, "cp-1", loaded.ID)

	latest, err := store.LoadLatest(context.Background(), "t1")
	require.NoError(t, err)
	assert.Equal(t, "cp-2", latest.ID)

	versionOne, err := store.LoadVersion(context.Background(), "t1", 1)
	require.NoError(t, err)
	assert.Equal(t, "cp-1", versionOne.ID)

	versions, err := store.ListVersions(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, versions, 2)
	assert.Equal(t, 1, versions[0].Version)
	assert.Equal(t, 2, versions[1].Version)
}

func TestPostgreSQLCheckpointStore_Rollback(t *testing.T) {
	client := newMockPostgresClient()
	store := NewPostgreSQLCheckpointStore(client, zap.NewNop())

	cp := &agentpkg.Checkpoint{ID: "cp-1", ThreadID: "t1", AgentID: "a1", State: agentpkg.StateReady, CreatedAt: time.Now()}
	require.NoError(t, store.Save(context.Background(), cp))

	require.NoError(t, store.Rollback(context.Background(), "t1", 1))

	versions, err := store.ListVersions(context.Background(), "t1")
	require.NoError(t, err)
	require.Len(t, versions, 2)

	rolledBack, err := store.LoadVersion(context.Background(), "t1", 2)
	require.NoError(t, err)
	assert.Equal(t, 2, rolledBack.Version)
	assert.Equal(t, float64(1), rolledBack.Metadata["rollback_from_version"])
}
