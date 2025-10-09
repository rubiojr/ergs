package cmd

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/db"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

// MigrateCommand creates the migrate command
func MigrateCommand() *cli.Command {
	return &cli.Command{
		Name:  "migrate",
		Usage: "Run database migrations",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "status",
				Usage: "Show migration status without applying migrations",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "datasource",
				Usage: "Apply migrations to a specific datasource only",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return RunMigrations(c.String("config"), c.Bool("status"), c.String("datasource"))
		},
	}
}

// RunMigrations handles the migration process (exported for testing)
func RunMigrations(configPath string, statusOnly bool, datasourceName string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	registry := core.GetGlobalRegistry()

	if err := createDatasourcesFromConfig(registry, cfg); err != nil {
		return fmt.Errorf("creating datasources: %w", err)
	}
	defer func() {
		if err := registry.Close(); err != nil {
			fmt.Printf("Warning: failed to close registry: %v\n", err)
		}
	}()

	storageManager := storage.NewManagerWithoutMigrationCheck(cfg.StorageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	datasources := registry.GetAllDatasources()

	// If a specific datasource is requested, filter to only that one
	if datasourceName != "" {
		if _, exists := datasources[datasourceName]; exists {
			datasources = map[string]core.Datasource{datasourceName: datasources[datasourceName]}
		} else {
			return fmt.Errorf("datasource '%s' not found", datasourceName)
		}
	}

	// Run migrations for each datasource database
	for name := range datasources {
		// Skip importer datasource (it has no dedicated persistent database / schema)
		if name == "importer" {
			continue
		}
		fmt.Printf("\n=== Datasource: %s ===\n", name)

		dbPath := filepath.Join(cfg.StorageDir, fmt.Sprintf("%s.db", name))

		// Check if database exists
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			fmt.Printf("Database does not exist, will be created on first use: %s\n", dbPath)
			continue
		}

		// Get storage for this datasource to access the database
		genericStorage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}

		// Get the database connection (we need to add a method to expose this)
		dbConn, err := getDBConnection(genericStorage)
		if err != nil {
			return fmt.Errorf("getting database connection for %s: %w", name, err)
		}

		migrationManager := db.NewMigrationManager(dbConn)

		if statusOnly {
			if err := showMigrationStatus(migrationManager, name); err != nil {
				return fmt.Errorf("showing migration status for %s: %w", name, err)
			}
		} else {
			if err := migrationManager.ApplyPendingMigrations(); err != nil {
				return fmt.Errorf("applying migrations for %s: %w", name, err)
			}
		}
	}

	if statusOnly {
		fmt.Println("\nMigration status check completed")
	} else {
		fmt.Println("\nAll migrations completed successfully")
	}

	return nil
}

// showMigrationStatus displays the current migration status
func showMigrationStatus(manager *db.MigrationManager, datasourceName string) error {
	status, err := manager.GetMigrationStatus()
	if err != nil {
		return err
	}

	fmt.Printf("Applied migrations: %d\n", len(status.Applied))
	for _, migration := range status.Applied {
		appliedTime := "unknown"
		if migration.AppliedAt != nil {
			appliedTime = migration.AppliedAt.Format("2006-01-02 15:04:05")
		}
		fmt.Printf("  ✓ %03d: %s (applied: %s)\n", migration.Version, migration.Name, appliedTime)
	}

	fmt.Printf("Pending migrations: %d\n", len(status.Pending))
	for _, migration := range status.Pending {
		fmt.Printf("  • %03d: %s\n", migration.Version, migration.Name)
	}

	if len(status.Pending) == 0 {
		fmt.Println("  (none - database is up to date)")
	}

	return nil
}

// getDBConnection extracts the database connection from GenericStorage
func getDBConnection(genericStorage *storage.GenericStorage) (*sql.DB, error) {
	return genericStorage.GetDB(), nil
}

// CheckPendingMigrations checks if there are pending migrations for any datasource
func CheckPendingMigrations(configPath string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	registry := core.GetGlobalRegistry()

	if err := createDatasourcesFromConfig(registry, cfg); err != nil {
		return fmt.Errorf("creating datasources: %w", err)
	}
	defer func() {
		if err := registry.Close(); err != nil {
			fmt.Printf("Warning: failed to close registry: %v\n", err)
		}
	}()

	storageManager := storage.NewManagerWithoutMigrationCheck(cfg.StorageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	datasources := registry.GetAllDatasources()

	// Check each datasource for pending migrations
	for name := range datasources {
		dbPath := filepath.Join(cfg.StorageDir, fmt.Sprintf("%s.db", name))

		// If database doesn't exist, no pending migrations
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			continue
		}

		// Get storage for this datasource to access the database
		genericStorage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}

		// Get the database connection
		dbConn, err := getDBConnection(genericStorage)
		if err != nil {
			return fmt.Errorf("getting database connection for %s: %w", name, err)
		}

		migrationManager := db.NewMigrationManager(dbConn)

		pending, err := migrationManager.GetPendingMigrations()
		if err != nil {
			return fmt.Errorf("checking pending migrations for %s: %w", name, err)
		}

		if len(pending) > 0 {
			return fmt.Errorf("database '%s' has %d pending migrations. Run 'ergs migrate' first", name, len(pending))
		}
	}

	return nil
}
