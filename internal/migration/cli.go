package migration

import (
	"context"
	"fmt"
	"io"
	"os"
	"text/tabwriter"
)

// CLI provides command-line interface functionality for migrations
type CLI struct {
	migrator Migrator
	output   io.Writer
}

// NewCLI creates a new CLI instance
func NewCLI(migrator Migrator) *CLI {
	return &CLI{
		migrator: migrator,
		output:   os.Stdout,
	}
}

// SetOutput sets the output writer for CLI messages
func (c *CLI) SetOutput(w io.Writer) {
	c.output = w
}

// RunUp runs all pending migrations
func (c *CLI) RunUp(ctx context.Context) error {
	fmt.Fprintln(c.output, "Running migrations...")

	if err := c.migrator.Up(ctx); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	info, err := c.migrator.Info(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.output, "Migrations complete. Current version: %d\n", info.CurrentVersion)
	return nil
}

// RunDown rolls back the last migration
func (c *CLI) RunDown(ctx context.Context) error {
	fmt.Fprintln(c.output, "Rolling back last migration...")

	if err := c.migrator.Down(ctx); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	info, err := c.migrator.Info(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.output, "Rollback complete. Current version: %d\n", info.CurrentVersion)
	return nil
}

// RunDownAll rolls back all migrations
func (c *CLI) RunDownAll(ctx context.Context) error {
	fmt.Fprintln(c.output, "Rolling back all migrations...")

	if err := c.migrator.DownAll(ctx); err != nil {
		return fmt.Errorf("rollback failed: %w", err)
	}

	fmt.Fprintln(c.output, "All migrations rolled back.")
	return nil
}

// RunSteps applies or rolls back n migrations
func (c *CLI) RunSteps(ctx context.Context, n int) error {
	if n > 0 {
		fmt.Fprintf(c.output, "Applying %d migration(s)...\n", n)
	} else {
		fmt.Fprintf(c.output, "Rolling back %d migration(s)...\n", -n)
	}

	if err := c.migrator.Steps(ctx, n); err != nil {
		return fmt.Errorf("migration steps failed: %w", err)
	}

	info, err := c.migrator.Info(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintf(c.output, "Complete. Current version: %d\n", info.CurrentVersion)
	return nil
}

// RunGoto migrates to a specific version
func (c *CLI) RunGoto(ctx context.Context, version uint) error {
	fmt.Fprintf(c.output, "Migrating to version %d...\n", version)

	if err := c.migrator.Goto(ctx, version); err != nil {
		return fmt.Errorf("migration failed: %w", err)
	}

	fmt.Fprintf(c.output, "Migration complete. Current version: %d\n", version)
	return nil
}

// RunForce forces the migration version
func (c *CLI) RunForce(ctx context.Context, version int) error {
	fmt.Fprintf(c.output, "Forcing version to %d...\n", version)

	if err := c.migrator.Force(ctx, version); err != nil {
		return fmt.Errorf("force failed: %w", err)
	}

	fmt.Fprintf(c.output, "Version forced to %d\n", version)
	return nil
}

// RunVersion shows the current migration version
func (c *CLI) RunVersion(ctx context.Context) error {
	version, dirty, err := c.migrator.Version(ctx)
	if err != nil {
		return fmt.Errorf("failed to get version: %w", err)
	}

	if version == 0 {
		fmt.Fprintln(c.output, "No migrations applied yet.")
		return nil
	}

	fmt.Fprintf(c.output, "Current version: %d", version)
	if dirty {
		fmt.Fprint(c.output, " (dirty)")
	}
	fmt.Fprintln(c.output)

	return nil
}

// RunStatus shows the status of all migrations
func (c *CLI) RunStatus(ctx context.Context) error {
	statuses, err := c.migrator.Status(ctx)
	if err != nil {
		return fmt.Errorf("failed to get status: %w", err)
	}

	if len(statuses) == 0 {
		fmt.Fprintln(c.output, "No migrations found.")
		return nil
	}

	// Print header
	w := tabwriter.NewWriter(c.output, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "VERSION\tNAME\tSTATUS")
	fmt.Fprintln(w, "-------\t----\t------")

	for _, s := range statuses {
		status := "Pending"
		if s.Applied {
			status = "Applied"
		}
		if s.Dirty {
			status = "Dirty"
		}
		fmt.Fprintf(w, "%06d\t%s\t%s\n", s.Version, s.Name, status)
	}

	w.Flush()

	// Print summary
	info, err := c.migrator.Info(ctx)
	if err != nil {
		return err
	}

	fmt.Fprintln(c.output)
	fmt.Fprintf(c.output, "Total: %d, Applied: %d, Pending: %d\n",
		info.TotalMigrations, info.AppliedMigrations, info.PendingMigrations)

	return nil
}

// RunInfo shows detailed migration information
func (c *CLI) RunInfo(ctx context.Context) error {
	info, err := c.migrator.Info(ctx)
	if err != nil {
		return fmt.Errorf("failed to get info: %w", err)
	}

	fmt.Fprintln(c.output, "Migration Information:")
	fmt.Fprintf(c.output, "  Current Version:    %d\n", info.CurrentVersion)
	fmt.Fprintf(c.output, "  Dirty:              %v\n", info.Dirty)
	fmt.Fprintf(c.output, "  Total Migrations:   %d\n", info.TotalMigrations)
	fmt.Fprintf(c.output, "  Applied Migrations: %d\n", info.AppliedMigrations)
	fmt.Fprintf(c.output, "  Pending Migrations: %d\n", info.PendingMigrations)

	return nil
}
