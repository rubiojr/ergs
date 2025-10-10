package integration_tests

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rubiojr/ergs/cmd"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

// TestServeCommandAbortsOnPendingMigrations verifies that the serve command
// aborts (returns an error) when there are pending migrations for a
// configured datasource database. This ensures that NewManager (with its
// optional datasource allow list) still enforces migration completeness for
// active datasources while stray (unconfigured) databases are ignored.
//
// Scenario:
//  1. Create a config with one datasource "timestamp".
//  2. Manually create timestamp.db with only the first migration version marked
//     as applied (simulating an outdated schema with pending migrations).
//  3. Run "serve" via the CLI command infrastructure.
//  4. Expect an error wrapping *storage.PendingMigrationsError.
func TestServeCommandAbortsOnPendingMigrations(t *testing.T) {
	tempDir := t.TempDir()

	// Write minimal config referencing the active datasource "timestamp"
	configPath := filepath.Join(tempDir, "config.toml")
	configContent := fmt.Sprintf(`
storage_dir = '%s'

[datasources]
[datasources.timestamp]
type = 'timestamp'
[datasources.timestamp.config]
interval_seconds = 1
`, tempDir)

	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	// Prepare a partial migration state for timestamp.db
	dbPath := filepath.Join(tempDir, "timestamp.db")
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		t.Fatalf("failed to open db: %v", err)
	}

	// Create migrations table and mark only the first migration as applied.
	// We assume migration version numbering starts at 1 and that more than one
	// migration exists in the embedded migrations set.
	_, err = db.Exec(`CREATE TABLE IF NOT EXISTS migrations (
		version INTEGER PRIMARY KEY,
		applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
	)`)
	if err != nil {
		t.Fatalf("failed to create migrations table: %v", err)
	}

	// Insert only version 1 (simulate outdated DB with pending migrations)
	_, err = db.Exec(`INSERT INTO migrations(version) VALUES (1)`)
	if err != nil {
		t.Fatalf("failed to insert initial migration version: %v", err)
	}

	if err := db.Close(); err != nil {
		t.Logf("warning: closing db: %v", err)
	}

	// Sanity: load config to ensure it's valid
	if _, err := config.LoadConfig(configPath); err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Build a minimal CLI root with just the serve command
	root := &cli.Command{
		Name: "ergs",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Value: configPath,
			},
		},
		Commands: []*cli.Command{
			cmd.ServeCommand(),
		},
	}

	// Run "serve" command; expect error due to pending migrations
	err = root.Run(context.Background(), []string{"ergs", "--config", configPath, "serve"})
	if err == nil {
		t.Fatalf("expected serve to fail due to pending migrations, but it succeeded")
	}

	// Validate error chain includes PendingMigrationsError
	var pmErr *storage.PendingMigrationsError
	if !errors.As(err, &pmErr) {
		t.Fatalf("expected PendingMigrationsError in chain, got: %v", err)
	}

	// Optional: confirm wrapped sentinel
	if !errors.Is(err, storage.ErrPendingMigrations) {
		t.Fatalf("expected error to wrap ErrPendingMigrations, got: %v", err)
	}

	// Datasource name should match configured datasource
	if pmErr.Datasource != "timestamp" {
		t.Errorf("expected datasource 'timestamp' in error, got '%s'", pmErr.Datasource)
	}

	// Count should be > 0 (there must be pending migrations)
	if pmErr.Count <= 0 {
		t.Errorf("expected pending migration count > 0, got %d", pmErr.Count)
	}
}
