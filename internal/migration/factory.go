package migration

import (
	"fmt"

	appconfig "github.com/BaSui01/agentflow/config"
)

// NewMigratorFromConfig creates a new migrator from application configuration
func NewMigratorFromConfig(cfg *appconfig.Config) (*DefaultMigrator, error) {
	if cfg == nil {
		return nil, fmt.Errorf("config is required")
	}

	return NewMigratorFromDatabaseConfig(cfg.Database)
}

// NewMigratorFromDatabaseConfig creates a new migrator from database configuration
func NewMigratorFromDatabaseConfig(dbCfg appconfig.DatabaseConfig) (*DefaultMigrator, error) {
	// Parse database type (Driver field in config)
	dbType, err := ParseDatabaseType(dbCfg.Driver)
	if err != nil {
		return nil, fmt.Errorf("invalid database type: %w", err)
	}

	// Build database URL
	var dbURL string
	switch dbType {
	case DatabaseTypePostgres:
		dbURL = BuildDatabaseURL(
			dbType,
			dbCfg.Host,
			dbCfg.Port,
			dbCfg.Name,     // Name field in config
			dbCfg.User,     // User field in config
			dbCfg.Password,
			dbCfg.SSLMode,
		)
	case DatabaseTypeMySQL:
		dbURL = BuildDatabaseURL(
			dbType,
			dbCfg.Host,
			dbCfg.Port,
			dbCfg.Name,
			dbCfg.User,
			dbCfg.Password,
			"",
		)
	case DatabaseTypeSQLite:
		// For SQLite, the Name field contains the file path
		dbURL = BuildDatabaseURL(dbType, "", 0, dbCfg.Name, "", "", "")
	default:
		return nil, fmt.Errorf("unsupported database type: %s", dbType)
	}

	// Create migrator config
	migCfg := &Config{
		DatabaseType: dbType,
		DatabaseURL:  dbURL,
		TableName:    "schema_migrations",
	}

	return NewMigrator(migCfg)
}

// NewMigratorFromURL creates a new migrator from a database URL
func NewMigratorFromURL(dbType, dbURL string) (*DefaultMigrator, error) {
	dt, err := ParseDatabaseType(dbType)
	if err != nil {
		return nil, err
	}

	return NewMigrator(&Config{
		DatabaseType: dt,
		DatabaseURL:  dbURL,
		TableName:    "schema_migrations",
	})
}
