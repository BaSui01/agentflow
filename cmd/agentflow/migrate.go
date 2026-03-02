package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strconv"

	"github.com/BaSui01/agentflow/internal/app/bootstrap"
	"github.com/BaSui01/agentflow/pkg/migration"
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

type migratorFlags struct {
	configPath *string
	dbType     *string
	dbURL      *string
}

func registerMigratorFlags(fs *flag.FlagSet) migratorFlags {
	return migratorFlags{
		configPath: fs.String("config", "", "Path to config file"),
		dbType:     fs.String("db-type", "", "Database type (postgres, mysql, sqlite)"),
		dbURL:      fs.String("db-url", "", "Database connection URL"),
	}
}

// createMigrator creates a migrator from command line flags
func createMigrator(fs *flag.FlagSet, args []string) (*migration.DefaultMigrator, error) {
	flags := registerMigratorFlags(fs)

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	return buildMigrator(*flags.configPath, *flags.dbType, *flags.dbURL)
}

func buildMigrator(configPath, dbType, dbURL string) (*migration.DefaultMigrator, error) {
	if dbType != "" && dbURL != "" {
		return migration.NewMigratorFromURL(dbType, dbURL)
	}

	cfg, err := bootstrap.LoadAndValidateConfig(configPath)
	if err != nil {
		return nil, err
	}

	if dbType != "" {
		cfg.Database.Driver = dbType
	}

	return migration.NewMigratorFromDatabaseConfig(cfg.Database)
}

func fatalf(format string, args ...any) {
	fmt.Fprintf(os.Stderr, format+"\n", args...)
	os.Exit(1)
}

func runWithMigrator(
	migrator *migration.DefaultMigrator,
	action func(context.Context, *migration.CLI) error,
) error {
	defer migrator.Close()
	cli := migration.NewCLI(migrator)
	return action(context.Background(), cli)
}

func runMigratorCommand(
	flagSetName string,
	args []string,
	createFailureMessage string,
	runFailureMessage string,
	action func(context.Context, *migration.CLI) error,
) {
	fs := flag.NewFlagSet(flagSetName, flag.ExitOnError)
	migrator, err := createMigrator(fs, args)
	if err != nil {
		fatalf("%s: %v", createFailureMessage, err)
	}

	if err := runWithMigrator(migrator, action); err != nil {
		fatalf("%s: %v", runFailureMessage, err)
	}
}

// runMigrateUp applies all pending migrations
func runMigrateUp(args []string) {
	runMigratorCommand(
		"migrate up",
		args,
		"Failed to create migrator",
		"Migration failed",
		func(ctx context.Context, cli *migration.CLI) error {
			return cli.RunUp(ctx)
		},
	)
}

// runMigrateDown rolls back the last migration
func runMigrateDown(args []string) {
	fs := flag.NewFlagSet("migrate down", flag.ExitOnError)
	all := fs.Bool("all", false, "Rollback all migrations")
	flags := registerMigratorFlags(fs)

	if err := fs.Parse(args); err != nil {
		fatalf("Failed to parse flags: %v", err)
	}

	migrator, err := buildMigrator(*flags.configPath, *flags.dbType, *flags.dbURL)
	if err != nil {
		fatalf("Failed to create migrator: %v", err)
	}

	err = runWithMigrator(migrator, func(ctx context.Context, cli *migration.CLI) error {
		if *all {
			return cli.RunDownAll(ctx)
		}
		return cli.RunDown(ctx)
	})
	if err != nil {
		fatalf("Migration rollback failed: %v", err)
	}
}

// runMigrateStatus shows the status of all migrations
func runMigrateStatus(args []string) {
	runMigratorCommand(
		"migrate status",
		args,
		"Failed to create migrator",
		"Failed to get status",
		func(ctx context.Context, cli *migration.CLI) error {
			return cli.RunStatus(ctx)
		},
	)
}

// runMigrateVersion shows the current migration version
func runMigrateVersion(args []string) {
	runMigratorCommand(
		"migrate version",
		args,
		"Failed to create migrator",
		"Failed to get version",
		func(ctx context.Context, cli *migration.CLI) error {
			return cli.RunVersion(ctx)
		},
	)
}

// runMigrateGoto migrates to a specific version
func runMigrateGoto(args []string) {
	if len(args) < 1 {
		fatalf("Usage: agentflow migrate goto <version>")
	}

	version, err := strconv.ParseUint(args[0], 10, 32)
	if err != nil {
		fatalf("Invalid version number: %s", args[0])
	}

	runMigratorCommand(
		"migrate goto",
		args[1:],
		"Failed to create migrator",
		"Migration failed",
		func(ctx context.Context, cli *migration.CLI) error {
			return cli.RunGoto(ctx, uint(version))
		},
	)
}

// runMigrateForce forces the migration version
func runMigrateForce(args []string) {
	if len(args) < 1 {
		fatalf("Usage: agentflow migrate force <version>")
	}

	version, err := strconv.ParseInt(args[0], 10, 32)
	if err != nil {
		fatalf("Invalid version number: %s", args[0])
	}

	runMigratorCommand(
		"migrate force",
		args[1:],
		"Failed to create migrator",
		"Force failed",
		func(ctx context.Context, cli *migration.CLI) error {
			return cli.RunForce(ctx, int(version))
		},
	)
}

// runMigrateReset rolls back all migrations
func runMigrateReset(args []string) {
	runMigratorCommand(
		"migrate reset",
		args,
		"Failed to create migrator",
		"Reset failed",
		func(ctx context.Context, cli *migration.CLI) error {
			return cli.RunDownAll(ctx)
		},
	)
}
