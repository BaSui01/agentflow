package memory

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// DBClient abstracts SQL database operations.
// Compatible with agent.PostgreSQLClient — the composition root wires the same
// adapter used by PostgreSQLCheckpointStore.
type DBClient interface {
	Exec(ctx context.Context, query string, args ...any) error
	Query(ctx context.Context, query string, args ...any) (DBRows, error)
}

// DBRows abstracts database row iteration.
type DBRows interface {
	Next() bool
	Scan(dest ...any) error
	Close() error
}

const createObservationsTable = `
CREATE TABLE IF NOT EXISTS agent_observations (
	id         TEXT PRIMARY KEY,
	agent_id   TEXT NOT NULL,
	date       TEXT NOT NULL,
	content    TEXT NOT NULL,
	created_at TIMESTAMP WITH TIME ZONE NOT NULL,
	metadata   JSONB
)`

const indexObsAgentID = `CREATE INDEX IF NOT EXISTS idx_observations_agent_id ON agent_observations(agent_id)`
const indexObsCreatedAt = `CREATE INDEX IF NOT EXISTS idx_observations_agent_created ON agent_observations(agent_id, created_at DESC)`

// PostgreSQLObservationStore persists observations in PostgreSQL.
// Reuses the project's agent.PostgreSQLClient interface for consistency
// with PostgreSQLCheckpointStore.
type PostgreSQLObservationStore struct {
	db DBClient
}

// NewPostgreSQLObservationStore creates a store and initializes the schema.
func NewPostgreSQLObservationStore(ctx context.Context, db DBClient) (*PostgreSQLObservationStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	if err := db.Exec(ctx, createObservationsTable); err != nil {
		return nil, fmt.Errorf("failed to create agent_observations table: %w", err)
	}
	if err := db.Exec(ctx, indexObsAgentID); err != nil {
		return nil, fmt.Errorf("failed to create agent_id index: %w", err)
	}
	if err := db.Exec(ctx, indexObsCreatedAt); err != nil {
		return nil, fmt.Errorf("failed to create created_at index: %w", err)
	}
	return &PostgreSQLObservationStore{db: db}, nil
}

func (s *PostgreSQLObservationStore) Save(ctx context.Context, obs Observation) error {
	var metadataJSON *string
	if len(obs.Metadata) > 0 {
		raw, err := json.Marshal(obs.Metadata)
		if err != nil {
			return fmt.Errorf("failed to marshal metadata: %w", err)
		}
		str := string(raw)
		metadataJSON = &str
	}

	query := `
		INSERT INTO agent_observations (id, agent_id, date, content, created_at, metadata)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT (id) DO UPDATE SET
			content = EXCLUDED.content,
			metadata = EXCLUDED.metadata`

	return s.db.Exec(ctx, query, obs.ID, obs.AgentID, obs.Date, obs.Content, obs.CreatedAt.UTC(), metadataJSON)
}

func (s *PostgreSQLObservationStore) LoadRecent(ctx context.Context, agentID string, limit int) ([]Observation, error) {
	query := `
		SELECT id, agent_id, date, content, created_at, metadata
		FROM agent_observations
		WHERE agent_id = $1
		ORDER BY created_at DESC
		LIMIT $2`

	rows, err := s.db.Query(ctx, query, agentID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObservations(rows)
}

func (s *PostgreSQLObservationStore) LoadByDateRange(ctx context.Context, agentID string, start, end time.Time) ([]Observation, error) {
	query := `
		SELECT id, agent_id, date, content, created_at, metadata
		FROM agent_observations
		WHERE agent_id = $1 AND created_at >= $2 AND created_at <= $3
		ORDER BY created_at ASC`

	rows, err := s.db.Query(ctx, query, agentID, start.UTC(), end.UTC())
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanObservations(rows)
}

func scanObservations(rows DBRows) ([]Observation, error) {
	var results []Observation
	for rows.Next() {
		var obs Observation
		var metadataJSON *string
		if err := rows.Scan(&obs.ID, &obs.AgentID, &obs.Date, &obs.Content, &obs.CreatedAt, &metadataJSON); err != nil {
			return nil, fmt.Errorf("failed to scan observation: %w", err)
		}
		if metadataJSON != nil && *metadataJSON != "" {
			if err := json.Unmarshal([]byte(*metadataJSON), &obs.Metadata); err != nil {
				return nil, fmt.Errorf("failed to unmarshal metadata: %w", err)
			}
		}
		results = append(results, obs)
	}
	return results, nil
}
