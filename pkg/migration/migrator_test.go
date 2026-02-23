package migration

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	_ "modernc.org/sqlite" // register pure-Go SQLite driver
)

func TestParseDatabaseType(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected DatabaseType
		wantErr  bool
	}{
		{"postgres", "postgres", DatabaseTypePostgres, false},
		{"postgresql", "postgresql", DatabaseTypePostgres, false},
		{"pg", "pg", DatabaseTypePostgres, false},
		{"mysql", "mysql", DatabaseTypeMySQL, false},
		{"mariadb", "mariadb", DatabaseTypeMySQL, false},
		{"sqlite", "sqlite", DatabaseTypeSQLite, false},
		{"sqlite3", "sqlite3", DatabaseTypeSQLite, false},
		{"uppercase", "POSTGRES", DatabaseTypePostgres, false},
		{"invalid", "invalid", "", true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := ParseDatabaseType(tt.input)
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestBuildDatabaseURL(t *testing.T) {
	tests := []struct {
		name     string
		dbType   DatabaseType
		host     string
		port     int
		database string
		username string
		password string
		sslMode  string
		expected string
	}{
		{
			name:     "postgres",
			dbType:   DatabaseTypePostgres,
			host:     "localhost",
			port:     5432,
			database: "testdb",
			username: "user",
			password: "pass",
			sslMode:  "disable",
			expected: "postgres://user:pass@localhost:5432/testdb?sslmode=disable",
		},
		{
			name:     "postgres_default_ssl",
			dbType:   DatabaseTypePostgres,
			host:     "localhost",
			port:     5432,
			database: "testdb",
			username: "user",
			password: "pass",
			sslMode:  "",
			expected: "postgres://user:pass@localhost:5432/testdb?sslmode=require",
		},
		{
			name:     "mysql",
			dbType:   DatabaseTypeMySQL,
			host:     "localhost",
			port:     3306,
			database: "testdb",
			username: "user",
			password: "pass",
			expected: "user:pass@tcp(localhost:3306)/testdb?parseTime=true&multiStatements=true",
		},
		{
			name:     "sqlite",
			dbType:   DatabaseTypeSQLite,
			database: "/path/to/db.sqlite",
			expected: "file:/path/to/db.sqlite?mode=rwc&_foreign_keys=on",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := BuildDatabaseURL(tt.dbType, tt.host, tt.port, tt.database, tt.username, tt.password, tt.sslMode)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestGetMigrationsPath(t *testing.T) {
	tests := []struct {
		dbType   DatabaseType
		expected string
	}{
		{DatabaseTypePostgres, filepath.Join("migrations", "postgres")},
		{DatabaseTypeMySQL, filepath.Join("migrations", "mysql")},
		{DatabaseTypeSQLite, filepath.Join("migrations", "sqlite")},
	}

	for _, tt := range tests {
		t.Run(string(tt.dbType), func(t *testing.T) {
			result := GetMigrationsPath(tt.dbType)
			assert.Equal(t, tt.expected, result)
		})
	}
}

func TestNewMigrator_InvalidConfig(t *testing.T) {
	// Test nil config
	_, err := NewMigrator(nil)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")

	// Test empty database URL
	_, err = NewMigrator(&Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "",
	})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "database URL is required")
}

func TestMigrator_SQLite_Integration(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Create migrator
	cfg := &Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
		TableName:    "schema_migrations",
	}

	migrator, err := NewMigrator(cfg)
	require.NoError(t, err)
	defer migrator.Close()

	ctx := context.Background()

	// Test initial version
	version, dirty, err := migrator.Version(ctx)
	require.NoError(t, err)
	assert.Equal(t, uint(0), version)
	assert.False(t, dirty)

	// Test Up
	err = migrator.Up(ctx)
	require.NoError(t, err)

	// Verify version after migration
	version, dirty, err = migrator.Version(ctx)
	require.NoError(t, err)
	assert.Greater(t, version, uint(0))
	assert.False(t, dirty)

	// Test Status
	statuses, err := migrator.Status(ctx)
	require.NoError(t, err)
	assert.NotEmpty(t, statuses)

	// Test Info
	info, err := migrator.Info(ctx)
	require.NoError(t, err)
	assert.Greater(t, info.CurrentVersion, uint(0))
	assert.Equal(t, info.TotalMigrations, info.AppliedMigrations)
	assert.Equal(t, 0, info.PendingMigrations)

	// Test Down
	err = migrator.Down(ctx)
	require.NoError(t, err)

	// Verify version after rollback
	newVersion, _, err := migrator.Version(ctx)
	require.NoError(t, err)
	assert.Less(t, newVersion, version)
}

func TestMigrator_GetAvailableMigrations(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires CGO in short mode")
	}

	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
		TableName:    "schema_migrations",
	}

	migrator, err := NewMigrator(cfg)
	require.NoError(t, err)
	defer migrator.Close()

	migrations, err := migrator.getAvailableMigrations()
	require.NoError(t, err)
	assert.NotEmpty(t, migrations)

	// Verify migrations are sorted by version
	for i := 1; i < len(migrations); i++ {
		assert.Greater(t, migrations[i].version, migrations[i-1].version)
	}
}

func TestCLI_Output(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping test that requires CGO in short mode")
	}

	// Create a temporary database file
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	cfg := &Config{
		DatabaseType: DatabaseTypeSQLite,
		DatabaseURL:  "file:" + dbPath + "?mode=rwc&_foreign_keys=on",
		TableName:    "schema_migrations",
	}

	migrator, err := NewMigrator(cfg)
	require.NoError(t, err)
	defer migrator.Close()

	cli := NewCLI(migrator)

	// Capture output
	r, w, _ := os.Pipe()
	cli.SetOutput(w)

	ctx := context.Background()

	// Run version command
	err = cli.RunVersion(ctx)
	require.NoError(t, err)

	w.Close()
	buf := make([]byte, 1024)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	assert.Contains(t, output, "No migrations applied yet")
}
