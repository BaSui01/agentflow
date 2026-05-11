package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/BaSui01/agentflow/types"
)

// EpisodicDBClient abstracts SQL database operations for episodic memory.
type EpisodicDBClient interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (EpisodicDBRows, error)
}

// EpisodicDBRows abstracts SQL row iteration for episodic memory.
type EpisodicDBRows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
}

const createEpisodicEventsTable = `
CREATE TABLE IF NOT EXISTS agent_episodic_events (
	id          TEXT PRIMARY KEY,
	agent_id    TEXT NOT NULL,
	type        TEXT NOT NULL,
	content     TEXT NOT NULL,
	context     JSONB,
	timestamp   TIMESTAMP WITH TIME ZONE NOT NULL,
	duration_ns BIGINT NOT NULL DEFAULT 0
)`

const indexEpisodicAgentTime = `CREATE INDEX IF NOT EXISTS idx_episodic_agent_time ON agent_episodic_events(agent_id, timestamp DESC)`
const indexEpisodicAgentType = `CREATE INDEX IF NOT EXISTS idx_episodic_agent_type ON agent_episodic_events(agent_id, type)`

// PostgreSQLEpisodicStore persists episodic memory events in PostgreSQL/TimescaleDB-compatible SQL.
type PostgreSQLEpisodicStore struct {
	db EpisodicDBClient
}

// NewPostgreSQLEpisodicStore creates a PostgreSQL-backed episodic store and initializes schema.
func NewPostgreSQLEpisodicStore(ctx context.Context, db EpisodicDBClient) (*PostgreSQLEpisodicStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	if err := db.Exec(ctx, createEpisodicEventsTable); err != nil {
		return nil, fmt.Errorf("failed to create agent_episodic_events table: %w", err)
	}
	if err := db.Exec(ctx, indexEpisodicAgentTime); err != nil {
		return nil, fmt.Errorf("failed to create episodic agent/time index: %w", err)
	}
	if err := db.Exec(ctx, indexEpisodicAgentType); err != nil {
		return nil, fmt.Errorf("failed to create episodic agent/type index: %w", err)
	}
	return &PostgreSQLEpisodicStore{db: db}, nil
}

func (s *PostgreSQLEpisodicStore) RecordEvent(ctx context.Context, event *types.EpisodicEvent) error {
	if event == nil {
		return fmt.Errorf("event is nil")
	}
	if event.ID == "" {
		event.ID = fmt.Sprintf("ep_%d", time.Now().UnixNano())
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = time.Now()
	}

	var contextJSON *string
	if len(event.Context) > 0 {
		raw, err := json.Marshal(event.Context)
		if err != nil {
			return fmt.Errorf("marshal episodic context: %w", err)
		}
		str := string(raw)
		contextJSON = &str
	}

	query := `
INSERT INTO agent_episodic_events (id, agent_id, type, content, context, timestamp, duration_ns)
VALUES ($1, $2, $3, $4, $5, $6, $7)
ON CONFLICT (id) DO UPDATE SET
	agent_id = EXCLUDED.agent_id,
	type = EXCLUDED.type,
	content = EXCLUDED.content,
	context = EXCLUDED.context,
	timestamp = EXCLUDED.timestamp,
	duration_ns = EXCLUDED.duration_ns`

	return s.db.Exec(ctx, query,
		event.ID,
		event.AgentID,
		event.Type,
		event.Content,
		contextJSON,
		event.Timestamp.UTC(),
		int64(event.Duration),
	)
}

func (s *PostgreSQLEpisodicStore) QueryEvents(ctx context.Context, query EpisodicQuery) ([]types.EpisodicEvent, error) {
	where, args := buildEpisodicWhere(query.AgentID, query.Type, query.StartTime, query.EndTime)
	limit := query.Limit
	if limit <= 0 {
		limit = 100
	}
	args = append(args, limit)

	sql := fmt.Sprintf(`
SELECT id, agent_id, type, content, context, timestamp, duration_ns
FROM agent_episodic_events
WHERE %s
ORDER BY timestamp DESC
LIMIT $%d`, strings.Join(where, " AND "), len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEpisodicRows(rows)
}

func (s *PostgreSQLEpisodicStore) GetTimeline(ctx context.Context, agentID string, start, end time.Time) ([]types.EpisodicEvent, error) {
	where, args := buildEpisodicWhere(agentID, "", start, end)
	args = append(args, 1000)

	sql := fmt.Sprintf(`
SELECT id, agent_id, type, content, context, timestamp, duration_ns
FROM agent_episodic_events
WHERE %s
ORDER BY timestamp ASC
LIMIT $%d`, strings.Join(where, " AND "), len(args))

	rows, err := s.db.Query(ctx, sql, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanEpisodicRows(rows)
}

func buildEpisodicWhere(agentID, eventType string, start, end time.Time) ([]string, []any) {
	where := []string{"1=1"}
	args := make([]any, 0, 4)
	add := func(clause string, arg any) {
		args = append(args, arg)
		where = append(where, fmt.Sprintf(clause, len(args)))
	}
	if agentID != "" {
		add("agent_id = $%d", agentID)
	}
	if eventType != "" {
		add("type = $%d", eventType)
	}
	if !start.IsZero() {
		add("timestamp >= $%d", start.UTC())
	}
	if !end.IsZero() {
		add("timestamp <= $%d", end.UTC())
	}
	return where, args
}

func scanEpisodicRows(rows EpisodicDBRows) ([]types.EpisodicEvent, error) {
	events := []types.EpisodicEvent{}
	for rows.Next() {
		var (
			event       types.EpisodicEvent
			contextJSON *string
			durationNS  int64
		)
		if err := rows.Scan(
			&event.ID,
			&event.AgentID,
			&event.Type,
			&event.Content,
			&contextJSON,
			&event.Timestamp,
			&durationNS,
		); err != nil {
			return nil, err
		}
		if contextJSON != nil && *contextJSON != "" {
			if err := json.Unmarshal([]byte(*contextJSON), &event.Context); err != nil {
				return nil, fmt.Errorf("unmarshal episodic context: %w", err)
			}
		}
		event.Duration = time.Duration(durationNS)
		events = append(events, event)
	}
	return events, nil
}
