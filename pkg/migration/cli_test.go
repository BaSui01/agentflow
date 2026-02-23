package migration

import (
	"bytes"
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// ============================================================
// Mock Migrator for CLI tests
// ============================================================

type mockMigrator struct {
	upFn      func(ctx context.Context) error
	downFn    func(ctx context.Context) error
	downAllFn func(ctx context.Context) error
	stepsFn   func(ctx context.Context, n int) error
	gotoFn    func(ctx context.Context, version uint) error
	forceFn   func(ctx context.Context, version int) error
	versionFn func(ctx context.Context) (uint, bool, error)
	statusFn  func(ctx context.Context) ([]MigrationStatus, error)
	infoFn    func(ctx context.Context) (*MigrationInfo, error)
	closeFn   func() error
}

func (m *mockMigrator) Up(ctx context.Context) error             { return m.upFn(ctx) }
func (m *mockMigrator) Down(ctx context.Context) error           { return m.downFn(ctx) }
func (m *mockMigrator) DownAll(ctx context.Context) error        { return m.downAllFn(ctx) }
func (m *mockMigrator) Steps(ctx context.Context, n int) error   { return m.stepsFn(ctx, n) }
func (m *mockMigrator) Goto(ctx context.Context, v uint) error   { return m.gotoFn(ctx, v) }
func (m *mockMigrator) Force(ctx context.Context, v int) error   { return m.forceFn(ctx, v) }
func (m *mockMigrator) Close() error                             { return m.closeFn() }

func (m *mockMigrator) Version(ctx context.Context) (uint, bool, error) {
	return m.versionFn(ctx)
}
func (m *mockMigrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	return m.statusFn(ctx)
}
func (m *mockMigrator) Info(ctx context.Context) (*MigrationInfo, error) {
	return m.infoFn(ctx)
}

func defaultMockInfo() *MigrationInfo {
	return &MigrationInfo{
		CurrentVersion:    3,
		Dirty:             false,
		TotalMigrations:   5,
		AppliedMigrations: 3,
		PendingMigrations: 2,
	}
}

// ============================================================
// CLI — RunUp
// ============================================================

func TestCLI_RunUp_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		upFn: func(ctx context.Context) error { return nil },
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return defaultMockInfo(), nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunUp(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Running migrations")
	assert.Contains(t, buf.String(), "Current version: 3")
}

func TestCLI_RunUp_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		upFn: func(ctx context.Context) error { return errors.New("up failed") },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunUp(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "migration failed")
}

// ============================================================
// CLI — RunDown
// ============================================================

func TestCLI_RunDown_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		downFn: func(ctx context.Context) error { return nil },
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return &MigrationInfo{CurrentVersion: 2}, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunDown(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Rolling back")
	assert.Contains(t, buf.String(), "Current version: 2")
}

func TestCLI_RunDown_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		downFn: func(ctx context.Context) error { return errors.New("down failed") },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunDown(context.Background())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "rollback failed")
}

// ============================================================
// CLI — RunDownAll
// ============================================================

func TestCLI_RunDownAll_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		downAllFn: func(ctx context.Context) error { return nil },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunDownAll(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "All migrations rolled back")
}

func TestCLI_RunDownAll_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		downAllFn: func(ctx context.Context) error { return errors.New("fail") },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunDownAll(context.Background())
	require.Error(t, err)
}

// ============================================================
// CLI — RunSteps
// ============================================================

func TestCLI_RunSteps_Forward(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		stepsFn: func(ctx context.Context, n int) error {
			assert.Equal(t, 2, n)
			return nil
		},
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return &MigrationInfo{CurrentVersion: 5}, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunSteps(context.Background(), 2)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Applying 2")
}

func TestCLI_RunSteps_Backward(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		stepsFn: func(ctx context.Context, n int) error {
			assert.Equal(t, -1, n)
			return nil
		},
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return &MigrationInfo{CurrentVersion: 2}, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunSteps(context.Background(), -1)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Rolling back 1")
}

func TestCLI_RunSteps_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		stepsFn: func(ctx context.Context, n int) error { return errors.New("step fail") },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunSteps(context.Background(), 1)
	require.Error(t, err)
}

// ============================================================
// CLI — RunGoto
// ============================================================

func TestCLI_RunGoto_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		gotoFn: func(ctx context.Context, v uint) error {
			assert.Equal(t, uint(5), v)
			return nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunGoto(context.Background(), 5)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "version 5")
}

func TestCLI_RunGoto_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		gotoFn: func(ctx context.Context, v uint) error { return errors.New("goto fail") },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunGoto(context.Background(), 5)
	require.Error(t, err)
}

// ============================================================
// CLI — RunForce
// ============================================================

func TestCLI_RunForce_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		forceFn: func(ctx context.Context, v int) error {
			assert.Equal(t, 3, v)
			return nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunForce(context.Background(), 3)
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Version forced to 3")
}

func TestCLI_RunForce_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		forceFn: func(ctx context.Context, v int) error { return errors.New("force fail") },
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunForce(context.Background(), 3)
	require.Error(t, err)
}

// ============================================================
// CLI — RunVersion
// ============================================================

func TestCLI_RunVersion_NoMigrations(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		versionFn: func(ctx context.Context) (uint, bool, error) {
			return 0, false, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunVersion(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No migrations applied")
}

func TestCLI_RunVersion_WithVersion(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		versionFn: func(ctx context.Context) (uint, bool, error) {
			return 5, false, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunVersion(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Current version: 5")
	assert.NotContains(t, buf.String(), "dirty")
}

func TestCLI_RunVersion_Dirty(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		versionFn: func(ctx context.Context) (uint, bool, error) {
			return 3, true, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunVersion(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "dirty")
}

func TestCLI_RunVersion_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		versionFn: func(ctx context.Context) (uint, bool, error) {
			return 0, false, errors.New("version fail")
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunVersion(context.Background())
	require.Error(t, err)
}

// ============================================================
// CLI — RunStatus
// ============================================================

func TestCLI_RunStatus_NoMigrations(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		statusFn: func(ctx context.Context) ([]MigrationStatus, error) {
			return nil, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunStatus(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "No migrations found")
}

func TestCLI_RunStatus_WithMigrations(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		statusFn: func(ctx context.Context) ([]MigrationStatus, error) {
			return []MigrationStatus{
				{Version: 1, Name: "init_schema", Applied: true},
				{Version: 2, Name: "add_users", Applied: true},
				{Version: 3, Name: "add_roles", Applied: false},
			}, nil
		},
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return &MigrationInfo{
				TotalMigrations:   3,
				AppliedMigrations: 2,
				PendingMigrations: 1,
			}, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunStatus(context.Background())
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "init_schema")
	assert.Contains(t, output, "Applied")
	assert.Contains(t, output, "Pending")
	assert.Contains(t, output, "Total: 3")
}

func TestCLI_RunStatus_DirtyMigration(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		statusFn: func(ctx context.Context) ([]MigrationStatus, error) {
			return []MigrationStatus{
				{Version: 1, Name: "init", Applied: true, Dirty: true},
			}, nil
		},
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return &MigrationInfo{TotalMigrations: 1, AppliedMigrations: 1}, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunStatus(context.Background())
	require.NoError(t, err)
	assert.Contains(t, buf.String(), "Dirty")
}

// ============================================================
// CLI — RunInfo
// ============================================================

func TestCLI_RunInfo_Success(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return &MigrationInfo{
				CurrentVersion:    5,
				Dirty:             false,
				TotalMigrations:   10,
				AppliedMigrations: 5,
				PendingMigrations: 5,
			}, nil
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunInfo(context.Background())
	require.NoError(t, err)
	output := buf.String()
	assert.Contains(t, output, "Current Version:    5")
	assert.Contains(t, output, "Total Migrations:   10")
	assert.Contains(t, output, "Applied Migrations: 5")
	assert.Contains(t, output, "Pending Migrations: 5")
}

func TestCLI_RunInfo_Error(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	mock := &mockMigrator{
		infoFn: func(ctx context.Context) (*MigrationInfo, error) {
			return nil, errors.New("info fail")
		},
	}
	cli := NewCLI(mock)
	cli.SetOutput(&buf)

	err := cli.RunInfo(context.Background())
	require.Error(t, err)
}

// ============================================================
// BuildDatabaseURL — edge cases
// ============================================================

func TestBuildDatabaseURL_UnknownType(t *testing.T) {
	t.Parallel()
	result := BuildDatabaseURL(DatabaseType("unknown"), "host", 5432, "db", "user", "pass", "")
	assert.Equal(t, "", result)
}

// ============================================================
// ParseDatabaseType — mixed case
// ============================================================

func TestParseDatabaseType_MixedCase(t *testing.T) {
	t.Parallel()
	dt, err := ParseDatabaseType("PostgreSQL")
	require.NoError(t, err)
	assert.Equal(t, DatabaseTypePostgres, dt)
}


