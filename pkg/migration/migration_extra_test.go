package migration

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	appconfig "github.com/BaSui01/agentflow/config"
	_ "modernc.org/sqlite"
)

// =============================================================================
// Factory — NewMigratorFromConfig
// =============================================================================

func TestNewMigratorFromConfig_NilConfig(t *testing.T) {
	_, err := NewMigratorFromConfig(nil)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

func TestNewMigratorFromConfig_InvalidDriver(t *testing.T) {
	cfg := &appconfig.Config{
		Database: appconfig.DatabaseConfig{
			Driver: "invalid",
		},
	}
	_, err := NewMigratorFromConfig(cfg)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid database type")
}

func TestNewMigratorFromConfig_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &appconfig.Config{
		Database: appconfig.DatabaseConfig{
			Driver: "sqlite",
			Name:   dbPath,
		},
	}
	m, err := NewMigratorFromConfig(cfg)
	require.NoError(t, err)
	require.NotNil(t, m)
	t.Cleanup(func() { m.Close() })
}

// =============================================================================
// Factory — NewMigratorFromURL
// =============================================================================

func TestNewMigratorFromURL_InvalidType(t *testing.T) {
	_, err := NewMigratorFromURL("invalid", "some://url")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

func TestNewMigratorFromURL_SQLite(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	m, err := NewMigratorFromURL("sqlite", "file:"+dbPath+"?mode=rwc&_foreign_keys=on")
	require.NoError(t, err)
	require.NotNil(t, m)
	t.Cleanup(func() { m.Close() })
}

// =============================================================================
// Factory — NewMigratorFromDatabaseConfig with different DB types
// =============================================================================

func TestNewMigratorFromDatabaseConfig_Postgres(t *testing.T) {
	_, err := NewMigratorFromDatabaseConfig(appconfig.DatabaseConfig{
		Driver:   "postgres",
		Host:     "localhost",
		Port:     5432,
		Name:     "testdb",
		User:     "user",
		Password: "pass",
		SSLMode:  "disable",
	})
	// Expected to fail (no postgres running), but should get past URL building
	require.Error(t, err)
}

func TestNewMigratorFromDatabaseConfig_MySQL(t *testing.T) {
	_, err := NewMigratorFromDatabaseConfig(appconfig.DatabaseConfig{
		Driver:   "mysql",
		Host:     "localhost",
		Port:     3306,
		Name:     "testdb",
		User:     "user",
		Password: "pass",
	})
	// Expected to fail (no mysql running)
	require.Error(t, err)
}

// =============================================================================
// Migrator — DownAll, Steps, Goto, Force (SQLite integration)
// =============================================================================

func TestMigrator_SQLite_DownAll(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	m, err := NewMigrator(&Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	ctx := context.Background()

	require.NoError(t, m.Up(ctx))
	require.NoError(t, m.DownAll(ctx))

	v, _, err := m.Version(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint(0), v)
}

func TestMigrator_SQLite_Steps(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	m, err := NewMigrator(&Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	ctx := context.Background()

	require.NoError(t, m.Steps(ctx, 1))

	v, _, err := m.Version(ctx)
	require.NoError(t, err)
	assert.Greater(t, v, uint(0))
}

func TestMigrator_SQLite_Goto(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	m, err := NewMigrator(&Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	ctx := context.Background()

	migrations, err := m.getAvailableMigrations()
	require.NoError(t, err)
	require.NotEmpty(t, migrations)

	targetVersion := migrations[0].version
	require.NoError(t, m.Goto(ctx, targetVersion))

	v, _, err := m.Version(ctx)
	require.NoError(t, err)
	assert.Equal(t, targetVersion, v)
}

func TestMigrator_SQLite_Force(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	m, err := NewMigrator(&Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })

	ctx := context.Background()

	require.NoError(t, m.Force(ctx, 1))

	v, _, err := m.Version(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint(1), v)
}

// =============================================================================
// Migrator — openDatabase unsupported type
// =============================================================================

func TestNewMigrator_UnsupportedDatabaseType(t *testing.T) {
	_, err := NewMigrator(&Config{
		DatabaseType: "oracle",
		DatabaseURL:  "oracle://localhost/test",
	})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unsupported database type")
}

// =============================================================================
// Config defaults
// =============================================================================

func TestNewMigrator_DefaultTableName(t *testing.T) {
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	m, err := NewMigrator(&Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
	})
	require.NoError(t, err)
	t.Cleanup(func() { m.Close() })
	assert.Equal(t, "schema_migrations", m.config.TableName)
}

// =============================================================================
// Migrator — Close with nil migrate
// =============================================================================

func TestMigrator_Close_NilMigrate(t *testing.T) {
	m := &DefaultMigrator{
		config: &Config{},
	}
	err := m.Close()
	assert.NoError(t, err)
}

