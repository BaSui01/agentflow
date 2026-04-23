package core

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"
)

type DBClient interface {
	ExecContext(ctx context.Context, query string, args ...any) (sql.Result, error)
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
	QueryRowContext(ctx context.Context, query string, args ...any) *sql.Row
}

const createCheckpointsTable = `
CREATE TABLE IF NOT EXISTS workflow_checkpoints (
	id          TEXT PRIMARY KEY,
	workflow_id TEXT NOT NULL,
	thread_id   TEXT NOT NULL,
	version     INT NOT NULL,
	data        JSONB NOT NULL,
	created_at  TIMESTAMPTZ NOT NULL,
	updated_at  TIMESTAMPTZ NOT NULL
)`

const createCheckpointsIndex = `
CREATE INDEX IF NOT EXISTS idx_workflow_checkpoints_thread_version 
ON workflow_checkpoints(thread_id, version DESC)`

type PostgreSQLCheckpointStore struct {
	db DBClient
}

func NewPostgreSQLCheckpointStore(ctx context.Context, db DBClient) (*PostgreSQLCheckpointStore, error) {
	if db == nil {
		return nil, fmt.Errorf("db must not be nil")
	}
	if _, err := db.ExecContext(ctx, createCheckpointsTable); err != nil {
		return nil, fmt.Errorf("failed to create workflow_checkpoints table: %w", err)
	}
	if _, err := db.ExecContext(ctx, createCheckpointsIndex); err != nil {
		return nil, fmt.Errorf("failed to create thread_id index: %w", err)
	}
	return &PostgreSQLCheckpointStore{db: db}, nil
}

func (s *PostgreSQLCheckpointStore) Save(ctx context.Context, cp *EnhancedCheckpoint) error {
	data, err := json.Marshal(cp)
	if err != nil {
		return fmt.Errorf("marshal checkpoint: %w", err)
	}
	now := time.Now().UTC()
	if cp.CreatedAt.IsZero() {
		cp.CreatedAt = now
	}
	query := `
		INSERT INTO workflow_checkpoints (id, workflow_id, thread_id, version, data, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		ON CONFLICT (id) DO UPDATE SET
			data = EXCLUDED.data,
			updated_at = EXCLUDED.updated_at`
	_, err = s.db.ExecContext(ctx, query, cp.ID, cp.WorkflowID, cp.ThreadID, cp.Version, data, cp.CreatedAt, now)
	return err
}

func (s *PostgreSQLCheckpointStore) Load(ctx context.Context, checkpointID string) (*EnhancedCheckpoint, error) {
	query := `SELECT data FROM workflow_checkpoints WHERE id = $1`
	row := s.db.QueryRowContext(ctx, query, checkpointID)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("checkpoint not found: %s", checkpointID)
		}
		return nil, err
	}
	var cp EnhancedCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

func (s *PostgreSQLCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*EnhancedCheckpoint, error) {
	query := `
		SELECT data FROM workflow_checkpoints 
		WHERE thread_id = $1 
		ORDER BY version DESC 
		LIMIT 1`
	row := s.db.QueryRowContext(ctx, query, threadID)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("no checkpoints for thread: %s", threadID)
		}
		return nil, err
	}
	var cp EnhancedCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

func (s *PostgreSQLCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*EnhancedCheckpoint, error) {
	query := `SELECT data FROM workflow_checkpoints WHERE thread_id = $1 AND version = $2`
	row := s.db.QueryRowContext(ctx, query, threadID, version)
	var data []byte
	if err := row.Scan(&data); err != nil {
		if err == sql.ErrNoRows {
			return nil, fmt.Errorf("version %d not found", version)
		}
		return nil, err
	}
	var cp EnhancedCheckpoint
	if err := json.Unmarshal(data, &cp); err != nil {
		return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
	}
	return &cp, nil
}

func (s *PostgreSQLCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]*EnhancedCheckpoint, error) {
	query := `
		SELECT data FROM workflow_checkpoints 
		WHERE thread_id = $1 
		ORDER BY version ASC`
	rows, err := s.db.QueryContext(ctx, query, threadID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []*EnhancedCheckpoint
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			return nil, err
		}
		var cp EnhancedCheckpoint
		if err := json.Unmarshal(data, &cp); err != nil {
			return nil, fmt.Errorf("unmarshal checkpoint: %w", err)
		}
		result = append(result, &cp)
	}
	return result, rows.Err()
}

func (s *PostgreSQLCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	query := `DELETE FROM workflow_checkpoints WHERE id = $1`
	_, err := s.db.ExecContext(ctx, query, checkpointID)
	return err
}

// Cleanup removes expired checkpoints and trims excess versions per workflow.
// It deletes rows older than retention, then keeps only the most recent maxVersionsPerWorkflow
// versions for each workflow_id. Returns the total number of deleted rows.
func (s *PostgreSQLCheckpointStore) Cleanup(ctx context.Context, retention time.Duration, maxVersionsPerWorkflow int) (int64, error) {
	var totalDeleted int64

	if retention > 0 {
		cutoff := time.Now().UTC().Add(-retention)
		query := `DELETE FROM workflow_checkpoints WHERE created_at < $1`
		result, err := s.db.ExecContext(ctx, query, cutoff)
		if err != nil {
			return 0, fmt.Errorf("cleanup expired checkpoints: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return 0, fmt.Errorf("rows affected: %w", err)
		}
		totalDeleted += n
	}

	if maxVersionsPerWorkflow > 0 {
		query := `
			DELETE FROM workflow_checkpoints
			WHERE id IN (
				SELECT id FROM (
					SELECT id, ROW_NUMBER() OVER (
						PARTITION BY workflow_id ORDER BY version DESC
					) AS rn
					FROM workflow_checkpoints
				) ranked
				WHERE rn > $1
			)`
		result, err := s.db.ExecContext(ctx, query, maxVersionsPerWorkflow)
		if err != nil {
			return totalDeleted, fmt.Errorf("cleanup excess versions: %w", err)
		}
		n, err := result.RowsAffected()
		if err != nil {
			return totalDeleted, fmt.Errorf("rows affected: %w", err)
		}
		totalDeleted += n
	}

	return totalDeleted, nil
}
