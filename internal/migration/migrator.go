package migration

import (
	"context"
	"database/sql"
	"embed"
	"errors"
	"fmt"
	"io/fs"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/golang-migrate/migrate/v4"
	"github.com/golang-migrate/migrate/v4/database"
	"github.com/golang-migrate/migrate/v4/database/mysql"
	"github.com/golang-migrate/migrate/v4/database/postgres"
	"github.com/golang-migrate/migrate/v4/database/sqlite3"
	"github.com/golang-migrate/migrate/v4/source"
	"github.com/golang-migrate/migrate/v4/source/iofs"
)

// =============================================================================
// Embedded Migration Files
// =============================================================================

//go:embed migrations/postgres/*.sql
var postgresFS embed.FS

//go:embed migrations/mysql/*.sql
var mysqlFS embed.FS

//go:embed migrations/sqlite/*.sql
var sqliteFS embed.FS

// =============================================================================
// Types and Interfaces
// =============================================================================

// DatabaseType represents the type of database
type DatabaseType string

const (
	// DatabaseTypePostgres represents PostgreSQL database
	DatabaseTypePostgres DatabaseType = "postgres"
	// DatabaseTypeMySQL represents MySQL database
	DatabaseTypeMySQL DatabaseType = "mysql"
	// DatabaseTypeSQLite represents SQLite database
	DatabaseTypeSQLite DatabaseType = "sqlite"
)

// MigrationStatus represents the status of a migration
type MigrationStatus struct {
	Version   uint
	Name      string
	Applied   bool
	AppliedAt *time.Time
	Dirty     bool
}

// MigrationInfo contains information about the current migration state
type MigrationInfo struct {
	CurrentVersion uint
	Dirty          bool
	TotalMigrations int
	AppliedMigrations int
	PendingMigrations int
}

// Config holds the configuration for the migrator
type Config struct {
	// DatabaseType specifies the type of database (postgres, mysql, sqlite)
	DatabaseType DatabaseType

	// DatabaseURL is the connection string for the database
	// Format depends on database type:
	// - PostgreSQL: postgres://user:password@host:port/dbname?sslmode=disable
	// - MySQL: user:password@tcp(host:port)/dbname?parseTime=true
	// - SQLite: file:path/to/db.sqlite?mode=rwc
	DatabaseURL string

	// MigrationsPath is the path to migration files (optional, uses embedded by default)
	MigrationsPath string

	// TableName is the name of the migrations table (default: schema_migrations)
	TableName string

	// LockTimeout is the timeout for acquiring migration lock
	LockTimeout time.Duration
}

// Migrator defines the interface for database migrations
type Migrator interface {
	// Up applies all pending migrations
	Up(ctx context.Context) error

	// Down rolls back the last migration
	Down(ctx context.Context) error

	// DownAll rolls back all migrations
	DownAll(ctx context.Context) error

	// Steps applies or rolls back n migrations
	// Positive n applies migrations, negative n rolls back
	Steps(ctx context.Context, n int) error

	// Goto migrates to a specific version
	Goto(ctx context.Context, version uint) error

	// Force sets the migration version without running migrations
	Force(ctx context.Context, version int) error

	// Version returns the current migration version
	Version(ctx context.Context) (uint, bool, error)

	// Status returns the status of all migrations
	Status(ctx context.Context) ([]MigrationStatus, error)

	// Info returns information about the current migration state
	Info(ctx context.Context) (*MigrationInfo, error)

	// Close closes the migrator and releases resources
	Close() error
}

// =============================================================================
// Default Migrator Implementation
// =============================================================================

// DefaultMigrator implements the Migrator interface using golang-migrate
type DefaultMigrator struct {
	config   *Config
	migrate  *migrate.Migrate
	db       *sql.DB
	dbDriver database.Driver
}

// NewMigrator creates a new migrator instance
func NewMigrator(cfg *Config) (*DefaultMigrator, error) {
	if cfg == nil {
		return nil, errors.New("config is required")
	}

	if cfg.DatabaseURL == "" {
		return nil, errors.New("database URL is required")
	}

	if cfg.TableName == "" {
		cfg.TableName = "schema_migrations"
	}

	if cfg.LockTimeout == 0 {
		cfg.LockTimeout = 15 * time.Second
	}

	m := &DefaultMigrator{
		config: cfg,
	}

	if err := m.init(); err != nil {
		return nil, fmt.Errorf("failed to initialize migrator: %w", err)
	}

	return m, nil
}

// init initializes the migrator
func (m *DefaultMigrator) init() error {
	var err error

	// Open database connection
	m.db, err = m.openDatabase()
	if err != nil {
		return fmt.Errorf("failed to open database: %w", err)
	}

	// Create database driver
	m.dbDriver, err = m.createDatabaseDriver()
	if err != nil {
		return fmt.Errorf("failed to create database driver: %w", err)
	}

	// Create source driver
	sourceDriver, err := m.createSourceDriver()
	if err != nil {
		return fmt.Errorf("failed to create source driver: %w", err)
	}

	// Create migrate instance
	m.migrate, err = migrate.NewWithInstance("iofs", sourceDriver, string(m.config.DatabaseType), m.dbDriver)
	if err != nil {
		return fmt.Errorf("failed to create migrate instance: %w", err)
	}

	return nil
}

// openDatabase opens a database connection based on the database type
func (m *DefaultMigrator) openDatabase() (*sql.DB, error) {
	var driverName string

	switch m.config.DatabaseType {
	case DatabaseTypePostgres:
		driverName = "postgres"
	case DatabaseTypeMySQL:
		driverName = "mysql"
	case DatabaseTypeSQLite:
		driverName = "sqlite3"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", m.config.DatabaseType)
	}

	db, err := sql.Open(driverName, m.config.DatabaseURL)
	if err != nil {
		return nil, err
	}

	// Test connection
	if err := db.Ping(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	return db, nil
}

// createDatabaseDriver creates a database driver for golang-migrate
func (m *DefaultMigrator) createDatabaseDriver() (database.Driver, error) {
	switch m.config.DatabaseType {
	case DatabaseTypePostgres:
		return postgres.WithInstance(m.db, &postgres.Config{
			MigrationsTable: m.config.TableName,
		})
	case DatabaseTypeMySQL:
		return mysql.WithInstance(m.db, &mysql.Config{
			MigrationsTable: m.config.TableName,
		})
	case DatabaseTypeSQLite:
		return sqlite3.WithInstance(m.db, &sqlite3.Config{
			MigrationsTable: m.config.TableName,
		})
	default:
		return nil, fmt.Errorf("unsupported database type: %s", m.config.DatabaseType)
	}
}

// createSourceDriver creates a source driver for migration files
func (m *DefaultMigrator) createSourceDriver() (source.Driver, error) {
	var fsys fs.FS
	var path string

	switch m.config.DatabaseType {
	case DatabaseTypePostgres:
		fsys = postgresFS
		path = "migrations/postgres"
	case DatabaseTypeMySQL:
		fsys = mysqlFS
		path = "migrations/mysql"
	case DatabaseTypeSQLite:
		fsys = sqliteFS
		path = "migrations/sqlite"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", m.config.DatabaseType)
	}

	return iofs.New(fsys, path)
}

// Up applies all pending migrations
func (m *DefaultMigrator) Up(ctx context.Context) error {
	if err := m.migrate.Up(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration up failed: %w", err)
	}
	return nil
}

// Down rolls back the last migration
func (m *DefaultMigrator) Down(ctx context.Context) error {
	if err := m.migrate.Steps(-1); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration down failed: %w", err)
	}
	return nil
}

// DownAll rolls back all migrations
func (m *DefaultMigrator) DownAll(ctx context.Context) error {
	if err := m.migrate.Down(); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration down all failed: %w", err)
	}
	return nil
}

// Steps applies or rolls back n migrations
func (m *DefaultMigrator) Steps(ctx context.Context, n int) error {
	if err := m.migrate.Steps(n); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration steps failed: %w", err)
	}
	return nil
}

// Goto migrates to a specific version
func (m *DefaultMigrator) Goto(ctx context.Context, version uint) error {
	if err := m.migrate.Migrate(version); err != nil && !errors.Is(err, migrate.ErrNoChange) {
		return fmt.Errorf("migration goto failed: %w", err)
	}
	return nil
}

// Force sets the migration version without running migrations
func (m *DefaultMigrator) Force(ctx context.Context, version int) error {
	if err := m.migrate.Force(version); err != nil {
		return fmt.Errorf("migration force failed: %w", err)
	}
	return nil
}

// Version returns the current migration version
func (m *DefaultMigrator) Version(ctx context.Context) (uint, bool, error) {
	version, dirty, err := m.migrate.Version()
	if err != nil {
		if errors.Is(err, migrate.ErrNilVersion) {
			return 0, false, nil
		}
		return 0, false, fmt.Errorf("failed to get version: %w", err)
	}
	return version, dirty, nil
}

// Status returns the status of all migrations
func (m *DefaultMigrator) Status(ctx context.Context) ([]MigrationStatus, error) {
	// Get current version
	currentVersion, dirty, err := m.Version(ctx)
	if err != nil {
		return nil, err
	}

	// Get all available migrations
	migrations, err := m.getAvailableMigrations()
	if err != nil {
		return nil, err
	}

	// Build status list
	var statuses []MigrationStatus
	for _, mig := range migrations {
		status := MigrationStatus{
			Version: mig.version,
			Name:    mig.name,
			Applied: mig.version <= currentVersion,
			Dirty:   dirty && mig.version == currentVersion,
		}
		statuses = append(statuses, status)
	}

	return statuses, nil
}

// Info returns information about the current migration state
func (m *DefaultMigrator) Info(ctx context.Context) (*MigrationInfo, error) {
	currentVersion, dirty, err := m.Version(ctx)
	if err != nil {
		return nil, err
	}

	migrations, err := m.getAvailableMigrations()
	if err != nil {
		return nil, err
	}

	applied := 0
	for _, mig := range migrations {
		if mig.version <= currentVersion {
			applied++
		}
	}

	return &MigrationInfo{
		CurrentVersion:    currentVersion,
		Dirty:             dirty,
		TotalMigrations:   len(migrations),
		AppliedMigrations: applied,
		PendingMigrations: len(migrations) - applied,
	}, nil
}

// Close closes the migrator and releases resources
func (m *DefaultMigrator) Close() error {
	var errs []error

	if m.migrate != nil {
		sourceErr, dbErr := m.migrate.Close()
		if sourceErr != nil {
			errs = append(errs, sourceErr)
		}
		if dbErr != nil {
			errs = append(errs, dbErr)
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("failed to close migrator: %v", errs)
	}

	return nil
}

// migrationFile represents a migration file
type migrationFile struct {
	version uint
	name    string
}

// getAvailableMigrations returns all available migrations
func (m *DefaultMigrator) getAvailableMigrations() ([]migrationFile, error) {
	var fsys fs.FS
	var path string

	switch m.config.DatabaseType {
	case DatabaseTypePostgres:
		fsys = postgresFS
		path = "migrations/postgres"
	case DatabaseTypeMySQL:
		fsys = mysqlFS
		path = "migrations/mysql"
	case DatabaseTypeSQLite:
		fsys = sqliteFS
		path = "migrations/sqlite"
	default:
		return nil, fmt.Errorf("unsupported database type: %s", m.config.DatabaseType)
	}

	entries, err := fs.ReadDir(fsys, path)
	if err != nil {
		return nil, fmt.Errorf("failed to read migrations directory: %w", err)
	}

	seen := make(map[uint]bool)
	var migrations []migrationFile

	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}

		name := entry.Name()
		if !strings.HasSuffix(name, ".up.sql") {
			continue
		}

		// Parse version from filename (e.g., 000001_init_schema.up.sql)
		parts := strings.SplitN(name, "_", 2)
		if len(parts) < 2 {
			continue
		}

		version, err := strconv.ParseUint(parts[0], 10, 32)
		if err != nil {
			continue
		}

		if seen[uint(version)] {
			continue
		}
		seen[uint(version)] = true

		// Extract migration name
		migName := strings.TrimSuffix(parts[1], ".up.sql")

		migrations = append(migrations, migrationFile{
			version: uint(version),
			name:    migName,
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].version < migrations[j].version
	})

	return migrations, nil
}

// =============================================================================
// Helper Functions
// =============================================================================

// ParseDatabaseType parses a database type string
func ParseDatabaseType(s string) (DatabaseType, error) {
	switch strings.ToLower(s) {
	case "postgres", "postgresql", "pg":
		return DatabaseTypePostgres, nil
	case "mysql", "mariadb":
		return DatabaseTypeMySQL, nil
	case "sqlite", "sqlite3":
		return DatabaseTypeSQLite, nil
	default:
		return "", fmt.Errorf("unsupported database type: %s", s)
	}
}

// BuildDatabaseURL builds a database URL from components
func BuildDatabaseURL(dbType DatabaseType, host string, port int, database, username, password, sslMode string) string {
	switch dbType {
	case DatabaseTypePostgres:
		if sslMode == "" {
			sslMode = "require"
		}
		return fmt.Sprintf("postgres://%s:%s@%s:%d/%s?sslmode=%s",
			username, password, host, port, database, sslMode)
	case DatabaseTypeMySQL:
		return fmt.Sprintf("%s:%s@tcp(%s:%d)/%s?parseTime=true&multiStatements=true",
			username, password, host, port, database)
	case DatabaseTypeSQLite:
		return fmt.Sprintf("file:%s?mode=rwc&_foreign_keys=on", database)
	default:
		return ""
	}
}

// GetMigrationsPath returns the path to migration files for a database type
func GetMigrationsPath(dbType DatabaseType) string {
	return filepath.Join("migrations", string(dbType))
}
