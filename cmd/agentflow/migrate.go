package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/BaSui01/agentflow/config"
	"github.com/BaSui01/agentflow/internal/migration"
)

// =============================================================================
// Database Migration Commands
// =============================================================================

// runMigrate handles the migrate command and its subcommands
func runMigrate(args []string) {
	if len(args) < 1 {
		printMigrateUsage()
		os.Exit(1)
	}

	subcommand := args[0]
	subargs := args[1:]

	switch subcommand {
	case "up":
		runMigrateUp(subargs)
	case "down":
		runMigrateDown(subargs)
	case "status":
		runMigrateStatus(subargs)
	case "version":
		runMigrateVersion(subargs)
	case "goto":
		runMigrateGoto(subargs)
	case "force":
		runMigrateForce(subargs)
	case "reset":
		runMigrateReset(subargs)
	case "help", "-h", "--help":
		printMigrateUsage()
	default:
		fmt.Fprintf(os.Stderr, "Unknown migrate subcommand: %s\n", subcommand)
		printMigrateUsage()
		os.Exit(1)
	}
}

// printMigrateUsage prints the usage information for migrate command
func printMigrateUsage() {
	fmt.Println(`Database Migration Commands

Usage:
  agentflow migrate <subcommand> [options]

Subcommands:
  up        Apply all pending migrations
  down      Rollback the last migration
  status    Show migration status
  version   Show current migration version
  goto      Migrate to a specific version
  force     Force set migration version (use with caution)
  reset     Rollback all migrations
  help      Show this help message

Options:
  --config <path>     Path to configuration file (YAML)
  --db-type <type>    Database type: postgres, mysql, sqlite (default: from config)
  --db-url <url>      Database connection URL (default: from config)

Examples:
  agentflow migrate up
  agentflow migrate up --config /etc/agentflow/config.yaml
  agentflow migrate down
  agentflow migrate status
  agentflow migrate goto 1
  agentflow migrate force 0
  agentflow migrate reset`)
}

// createMigrator creates a migrator from command line flags
func createMigrator(fs *flag.FlagSet, args []string) (*migration.DefaultMigrator, error) {
	configPath := fs.String("config", "", "Path to config file")
	dbType := fs.String("db-type", "", "Database type (postgres, mysql, sqlite)")
	dbURL := fs.String("db-url", "", "Database connection URL")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// If db-type and db-url are provided, use them directly
	if *dbType != "" && *dbURL != "" {
		return migration.NewMigratorFromURL(*dbType, *dbURL)
	}

	// Otherwise, load from config
	loader := config.NewLoader()
	if *configPath != "" {
		loader = loader.WithConfigPath(*configPath)
	}

	cfg, err := loader.Load()
	if err != nil {
		return nil, fmt.Errorf("failed to load config: %w", err)
	}

	// Override database type if specified
	if *dbType != "" {
		cfg.Database.Driver = *dbType
	}

	return migration.NewMigratorFromDatabaseConfig(cfg.Database)
}

// runMigrateUp applies all pending migrations
func runMigrateUp(args []string) {
	fs := flag.NewFlagSet("migrate up", flag.ExitOnError)
	migrator, err := createMigrator(fs, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if err := cli.RunUp(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
}

// runMigrateDown rolls back the last migration
func runMigrateDown(args []string) {
	fs := flag.NewFlagSet("migrate down", flag.ExitOnError)
	all := fs.Bool("all", false, "Rollback all migrations")

	// Parse flags first to get --all
	configPath := fs.String("config", "", "Path to config file")
	dbType := fs.String("db-type", "", "Database type (postgres, mysql, sqlite)")
	dbURL := fs.String("db-url", "", "Database connection URL")

	if err := fs.Parse(args); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to parse flags: %v\n", err)
		os.Exit(1)
	}

	// Create migrator
	var migrator *migration.DefaultMigrator
	var err error

	if *dbType != "" && *dbURL != "" {
		migrator, err = migration.NewMigratorFromURL(*dbType, *dbURL)
	} else {
		loader := config.NewLoader()
		if *configPath != "" {
			loader = loader.WithConfigPath(*configPath)
		}

		cfg, loadErr := loader.Load()
		if loadErr != nil {
			fmt.Fprintf(os.Stderr, "Failed to load config: %v\n", loadErr)
			os.Exit(1)
		}

		if *dbType != "" {
			cfg.Database.Driver = *dbType
		}

		migrator, err = migration.NewMigratorFromDatabaseConfig(cfg.Database)
	}

	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if *all {
		if err := cli.RunDownAll(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Migration rollback failed: %v\n", err)
			os.Exit(1)
		}
	} else {
		if err := cli.RunDown(ctx); err != nil {
			fmt.Fprintf(os.Stderr, "Migration rollback failed: %v\n", err)
			os.Exit(1)
		}
	}
}

// runMigrateStatus shows the status of all migrations
func runMigrateStatus(args []string) {
	fs := flag.NewFlagSet("migrate status", flag.ExitOnError)
	migrator, err := createMigrator(fs, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if err := cli.RunStatus(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get status: %v\n", err)
		os.Exit(1)
	}
}

// runMigrateVersion shows the current migration version
func runMigrateVersion(args []string) {
	fs := flag.NewFlagSet("migrate version", flag.ExitOnError)
	migrator, err := createMigrator(fs, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if err := cli.RunVersion(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Failed to get version: %v\n", err)
		os.Exit(1)
	}
}

// runMigrateGoto migrates to a specific version
func runMigrateGoto(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: agentflow migrate goto <version>\n")
		os.Exit(1)
	}

	version, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid version number: %s\n", args[0])
		os.Exit(1)
	}

	fs := flag.NewFlagSet("migrate goto", flag.ExitOnError)
	migrator, err := createMigrator(fs, args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if err := cli.RunGoto(ctx, uint(version)); err != nil {
		fmt.Fprintf(os.Stderr, "Migration failed: %v\n", err)
		os.Exit(1)
	}
}

// runMigrateForce forces the migration version
func runMigrateForce(args []string) {
	if len(args) < 1 {
		fmt.Fprintf(os.Stderr, "Usage: agentflow migrate force <version>\n")
		os.Exit(1)
	}

	version, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Invalid version number: %s\n", args[0])
		os.Exit(1)
	}

	fs := flag.NewFlagSet("migrate force", flag.ExitOnError)
	migrator, err := createMigrator(fs, args[1:])
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if err := cli.RunForce(ctx, int(version)); err != nil {
		fmt.Fprintf(os.Stderr, "Force failed: %v\n", err)
		os.Exit(1)
	}
}

// runMigrateReset rolls back all migrations
func runMigrateReset(args []string) {
	fs := flag.NewFlagSet("migrate reset", flag.ExitOnError)
	migrator, err := createMigrator(fs, args)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Failed to create migrator: %v\n", err)
		os.Exit(1)
	}
	defer migrator.Close()

	cli := migration.NewCLI(migrator)
	ctx := context.Background()

	if err := cli.RunDownAll(ctx); err != nil {
		fmt.Fprintf(os.Stderr, "Reset failed: %v\n", err)
		os.Exit(1)
	}
}
