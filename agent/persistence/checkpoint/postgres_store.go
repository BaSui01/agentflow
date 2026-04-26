package checkpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	agentcore "github.com/BaSui01/agentflow/agent/core"
	"github.com/BaSui01/agentflow/pkg/database"
	"go.uber.org/zap"
)

// PostgreSQLCheckpointStore persists agent checkpoints in PostgreSQL.
type PostgreSQLCheckpointStore struct {
	db     database.PostgreSQLClient
	logger *zap.Logger
}

// NewPostgreSQLCheckpointStore creates a PostgreSQL-backed agent checkpoint store.
func NewPostgreSQLCheckpointStore(db database.PostgreSQLClient, logger *zap.Logger) *PostgreSQLCheckpointStore {
	return &PostgreSQLCheckpointStore{
		db:     db,
		logger: checkpointLogger(logger, "postgresql_checkpoint"),
	}
}

// Save persists a checkpoint.
func (s *PostgreSQLCheckpointStore) Save(ctx context.Context, checkpoint *Checkpoint) error {
	if checkpoint.Version == 0 {
		versions, err := s.ListVersions(ctx, checkpoint.ThreadID)
		if err == nil && len(versions) > 0 {
			maxVersion := 0
			for _, v := range versions {
				if v.Version > maxVersion {
					maxVersion = v.Version
				}
			}
			checkpoint.Version = maxVersion + 1
		} else {
			checkpoint.Version = 1
		}
	}

	data, err := json.Marshal(checkpoint)
	if err != nil {
		return fmt.Errorf("failed to marshal checkpoint: %w", err)
	}

	query := `
		INSERT INTO agent_checkpoints (id, thread_id, agent_id, version, state, data, created_at, parent_id)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		ON CONFLICT (id) DO UPDATE SET
			version = EXCLUDED.version,
			state = EXCLUDED.state,
			data = EXCLUDED.data
	`

	err = s.db.Exec(ctx, query,
		checkpoint.ID,
		checkpoint.ThreadID,
		checkpoint.AgentID,
		checkpoint.Version,
		checkpoint.State,
		data,
		checkpoint.CreatedAt,
		checkpoint.ParentID,
	)
	if err != nil {
		return fmt.Errorf("failed to save checkpoint: %w", err)
	}

	s.logger.Debug("checkpoint saved to postgresql",
		zap.String("checkpoint_id", checkpoint.ID),
		zap.String("thread_id", checkpoint.ThreadID),
		zap.Int("version", checkpoint.Version),
	)

	return nil
}

// Load retrieves a checkpoint by ID.
func (s *PostgreSQLCheckpointStore) Load(ctx context.Context, checkpointID string) (*Checkpoint, error) {
	query := `SELECT data FROM agent_checkpoints WHERE id = $1`

	var data []byte
	row := s.db.QueryRow(ctx, query, checkpointID)
	if err := row.Scan(&data); err != nil {
		return nil, fmt.Errorf("checkpoint not found: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// LoadLatest retrieves the latest checkpoint for a thread.
func (s *PostgreSQLCheckpointStore) LoadLatest(ctx context.Context, threadID string) (*Checkpoint, error) {
	query := `
		SELECT data FROM agent_checkpoints
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT 1
	`

	var data []byte
	row := s.db.QueryRow(ctx, query, threadID)
	if err := row.Scan(&data); err != nil {
		return nil, fmt.Errorf("no checkpoints found: %w", err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// List enumerates checkpoints for a thread.
func (s *PostgreSQLCheckpointStore) List(ctx context.Context, threadID string, limit int) ([]*Checkpoint, error) {
	query := `
		SELECT data FROM agent_checkpoints
		WHERE thread_id = $1
		ORDER BY created_at DESC
		LIMIT $2
	`

	rows, err := s.db.Query(ctx, query, threadID, limit)
	if err != nil {
		return nil, fmt.Errorf("query checkpoints: %w", err)
	}
	defer rows.Close()

	checkpoints := make([]*Checkpoint, 0)
	for rows.Next() {
		var data []byte
		if err := rows.Scan(&data); err != nil {
			s.logger.Warn("failed to scan row", zap.Error(err))
			continue
		}

		var checkpoint Checkpoint
		if err := json.Unmarshal(data, &checkpoint); err != nil {
			s.logger.Warn("failed to unmarshal checkpoint", zap.Error(err))
			continue
		}

		checkpoints = append(checkpoints, &checkpoint)
	}

	return checkpoints, nil
}

// Delete removes a checkpoint by ID.
func (s *PostgreSQLCheckpointStore) Delete(ctx context.Context, checkpointID string) error {
	query := `DELETE FROM agent_checkpoints WHERE id = $1`
	return s.db.Exec(ctx, query, checkpointID)
}

// DeleteThread removes all checkpoints for a thread.
func (s *PostgreSQLCheckpointStore) DeleteThread(ctx context.Context, threadID string) error {
	query := `DELETE FROM agent_checkpoints WHERE thread_id = $1`
	return s.db.Exec(ctx, query, threadID)
}

// LoadVersion retrieves a checkpoint by thread/version.
func (s *PostgreSQLCheckpointStore) LoadVersion(ctx context.Context, threadID string, version int) (*Checkpoint, error) {
	query := `
		SELECT data FROM agent_checkpoints
		WHERE thread_id = $1 AND version = $2
		LIMIT 1
	`

	var data []byte
	row := s.db.QueryRow(ctx, query, threadID, version)
	if err := row.Scan(&data); err != nil {
		return nil, fmt.Errorf("version %d not found: %w", version, err)
	}

	var checkpoint Checkpoint
	if err := json.Unmarshal(data, &checkpoint); err != nil {
		return nil, fmt.Errorf("failed to unmarshal checkpoint: %w", err)
	}

	return &checkpoint, nil
}

// ListVersions lists all checkpoint versions for a thread.
func (s *PostgreSQLCheckpointStore) ListVersions(ctx context.Context, threadID string) ([]CheckpointVersion, error) {
	query := `
		SELECT id, version, created_at, state FROM agent_checkpoints
		WHERE thread_id = $1
		ORDER BY version ASC
	`

	rows, err := s.db.Query(ctx, query, threadID)
	if err != nil {
		return nil, fmt.Errorf("query checkpoint versions: %w", err)
	}
	defer rows.Close()

	versions := make([]CheckpointVersion, 0)
	for rows.Next() {
		var v CheckpointVersion
		var state string
		if err := rows.Scan(&v.ID, &v.Version, &v.CreatedAt, &state); err != nil {
			s.logger.Warn("failed to scan version row", zap.Error(err))
			continue
		}
		v.State = agentcore.State(state)
		v.Summary = fmt.Sprintf("Checkpoint at %s", v.CreatedAt.Format(time.RFC3339))
		versions = append(versions, v)
	}

	return versions, nil
}

// Rollback creates a new checkpoint based on a historical version.
func (s *PostgreSQLCheckpointStore) Rollback(ctx context.Context, threadID string, version int) error {
	checkpoint, err := s.LoadVersion(ctx, threadID, version)
	if err != nil {
		return fmt.Errorf("load version %d for rollback: %w", version, err)
	}

	newCheckpoint := *checkpoint
	newCheckpoint.ID = nextCheckpointID()
	newCheckpoint.CreatedAt = time.Now()
	newCheckpoint.ParentID = checkpoint.ID

	versions, err := s.ListVersions(ctx, threadID)
	if err != nil {
		return fmt.Errorf("list versions for rollback: %w", err)
	}

	maxVersion := 0
	for _, v := range versions {
		if v.Version > maxVersion {
			maxVersion = v.Version
		}
	}

	newCheckpoint.Version = maxVersion + 1

	if newCheckpoint.Metadata == nil {
		newCheckpoint.Metadata = make(map[string]any)
	}
	newCheckpoint.Metadata["rollback_from_version"] = version

	return s.Save(ctx, &newCheckpoint)
}
