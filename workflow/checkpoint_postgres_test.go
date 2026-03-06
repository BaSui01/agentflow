package workflow

import (
	"context"
	"database/sql"
	"encoding/json"
	"testing"
	"time"

	"github.com/DATA-DOG/go-sqlmock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func newMockDB(t *testing.T) (*sql.DB, sqlmock.Sqlmock) {
	db, mock, err := sqlmock.New()
	require.NoError(t, err)
	return db, mock
}

func TestPostgreSQLCheckpointStore_New_NilDB(t *testing.T) {
	_, err := NewPostgreSQLCheckpointStore(context.Background(), nil)
	assert.Error(t, err)
}

func TestPostgreSQLCheckpointStore_SaveAndLoad(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workflow_checkpoints").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_workflow_checkpoints_thread_version").WillReturnResult(sqlmock.NewResult(0, 0))

	store, err := NewPostgreSQLCheckpointStore(ctx, db)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	cp := &EnhancedCheckpoint{
		ID:             "ckpt1",
		WorkflowID:     "wf1",
		ThreadID:       "thread1",
		Version:        1,
		NodeResults:    map[string]any{"n1": "result"},
		Variables:      map[string]any{"x": 1},
		CompletedNodes: []string{"n1"},
		CreatedAt:      time.Now().UTC(),
	}

	mock.ExpectExec("INSERT INTO workflow_checkpoints").WithArgs(
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
		sqlmock.AnyArg(), sqlmock.AnyArg(), sqlmock.AnyArg(),
	).WillReturnResult(sqlmock.NewResult(1, 1))

	err = store.Save(ctx, cp)
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())

	dataJSON, _ := json.Marshal(cp)
	mock.ExpectQuery("SELECT data FROM workflow_checkpoints WHERE id = \\$1").WithArgs("ckpt1").
		WillReturnRows(sqlmock.NewRows([]string{"data"}).AddRow(dataJSON))

	loaded, err := store.Load(ctx, "ckpt1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
	assert.Equal(t, "ckpt1", loaded.ID)
	assert.Equal(t, "wf1", loaded.WorkflowID)
	assert.Equal(t, "thread1", loaded.ThreadID)
	assert.Equal(t, 1, loaded.Version)
	assert.Equal(t, "result", loaded.NodeResults["n1"])
	assert.Equal(t, float64(1), loaded.Variables["x"])
}

func TestPostgreSQLCheckpointStore_Load_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workflow_checkpoints").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_workflow_checkpoints_thread_version").WillReturnResult(sqlmock.NewResult(0, 0))

	store, err := NewPostgreSQLCheckpointStore(ctx, db)
	require.NoError(t, err)

	mock.ExpectQuery("SELECT data FROM workflow_checkpoints WHERE id = \\$1").WithArgs("nonexistent").
		WillReturnError(sql.ErrNoRows)

	_, err = store.Load(ctx, "nonexistent")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "checkpoint not found")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQLCheckpointStore_LoadLatest_NotFound(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workflow_checkpoints").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_workflow_checkpoints_thread_version").WillReturnResult(sqlmock.NewResult(0, 0))

	store, err := NewPostgreSQLCheckpointStore(ctx, db)
	require.NoError(t, err)

	mock.ExpectQuery("SELECT data FROM workflow_checkpoints").WithArgs("thread1").
		WillReturnError(sql.ErrNoRows)

	_, err = store.LoadLatest(ctx, "thread1")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "no checkpoints for thread")
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQLCheckpointStore_Delete(t *testing.T) {
	db, mock := newMockDB(t)
	defer db.Close()

	ctx := context.Background()
	mock.ExpectExec("CREATE TABLE IF NOT EXISTS workflow_checkpoints").WillReturnResult(sqlmock.NewResult(0, 0))
	mock.ExpectExec("CREATE INDEX IF NOT EXISTS idx_workflow_checkpoints_thread_version").WillReturnResult(sqlmock.NewResult(0, 0))

	store, err := NewPostgreSQLCheckpointStore(ctx, db)
	require.NoError(t, err)

	mock.ExpectExec("DELETE FROM workflow_checkpoints WHERE id = \\$1").WithArgs("ckpt1").
		WillReturnResult(sqlmock.NewResult(0, 1))

	err = store.Delete(ctx, "ckpt1")
	require.NoError(t, err)
	require.NoError(t, mock.ExpectationsWereMet())
}

func TestPostgreSQLCheckpointStore_ImplementsCheckpointStore(t *testing.T) {
	var _ CheckpointStore = (*PostgreSQLCheckpointStore)(nil)
}
