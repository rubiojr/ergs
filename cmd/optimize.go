package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

// OptimizeCommand creates the optimize command
func OptimizeCommand() *cli.Command {
	return &cli.Command{
		Name:  "optimize",
		Usage: "Database optimization and maintenance commands",
		Commands: []*cli.Command{
			{
				Name:  "check",
				Usage: "Run integrity checks on all databases",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "datasource",
						Usage: "Target specific datasource (optional)",
					},
					&cli.BoolFlag{
						Name:  "quick",
						Usage: "Skip deep FTS5-specific integrity checks",
						Value: false,
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return checkDatabases(c.String("config"), c.String("datasource"), !c.Bool("quick"))
				},
			},
			{
				Name:  "fts-rebuild",
				Usage: "Rebuild FTS5 indexes for all databases",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "datasource",
						Usage: "Target specific datasource (optional)",
					},
					&cli.BoolFlag{
						Name:  "force",
						Usage: "Force rebuild without checking first (skips integrity check)",
						Value: false,
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return rebuildFTS(c.String("config"), c.String("datasource"), c.Bool("force"))
				},
			},
			{
				Name:  "analyze",
				Usage: "Run ANALYZE to update query planner statistics",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "datasource",
						Usage: "Target specific datasource (optional)",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return analyzeDatabase(c.String("config"), c.String("datasource"))
				},
			},
			{
				Name:  "vacuum",
				Usage: "Run VACUUM to defragment database",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "datasource",
						Usage: "Target specific datasource (optional)",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return vacuumDatabase(c.String("config"), c.String("datasource"))
				},
			},
			{
				Name:  "checkpoint",
				Usage: "Run WAL checkpoint to flush changes",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "datasource",
						Usage: "Target specific datasource (optional)",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return checkpointDatabase(c.String("config"), c.String("datasource"))
				},
			},
			{
				Name:  "all",
				Usage: "Run all optimization operations (analyze, checkpoint, optimize)",
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:  "datasource",
						Usage: "Target specific datasource (optional)",
					},
				},
				Action: func(ctx context.Context, c *cli.Command) error {
					return optimizeAll(c.String("config"), c.String("datasource"))
				},
			},
		},
	}
}

// optimizeAll runs all optimization operations
func optimizeAll(configPath string, datasourceName string) error {
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

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	if datasourceName != "" {
		fmt.Printf("Running all optimization operations on %s...\n", datasourceName)
	} else {
		fmt.Println("Running all optimization operations...")
	}
	fmt.Println()

	// Get datasources to process
	datasources := getDatasourcesToProcess(storageManager, datasourceName)
	if len(datasources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	// Run basic optimization
	fmt.Println("Running PRAGMA optimize...")
	for _, name := range datasources {
		fmt.Printf("  Optimizing %s...\n", name)
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}
		if err := storage.Optimize(); err != nil {
			return fmt.Errorf("optimizing %s: %w", name, err)
		}
	}
	fmt.Println("✓ PRAGMA optimize completed")
	fmt.Println()

	// Run ANALYZE
	fmt.Println("Running ANALYZE...")
	for _, name := range datasources {
		fmt.Printf("  Analyzing %s...\n", name)
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}
		if err := storage.Analyze(); err != nil {
			return fmt.Errorf("analyzing %s: %w", name, err)
		}
	}
	fmt.Println("✓ ANALYZE completed")
	fmt.Println()

	// Run WAL checkpoint
	fmt.Println("Running WAL checkpoint...")
	for _, name := range datasources {
		fmt.Printf("  Checkpointing %s...\n", name)
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}
		if err := storage.WALCheckpoint(); err != nil {
			return fmt.Errorf("checkpointing %s: %w", name, err)
		}
	}
	fmt.Println("✓ WAL checkpoint completed")

	fmt.Println()
	fmt.Println("All optimization operations completed successfully")
	return nil
}

// analyzeDatabase runs ANALYZE on all databases
func analyzeDatabase(configPath string, datasourceName string) error {
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

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	if datasourceName != "" {
		fmt.Printf("Running ANALYZE on %s...\n", datasourceName)
	} else {
		fmt.Println("Running ANALYZE on all databases...")
	}
	fmt.Println()

	// Get datasources to process
	datasources := getDatasourcesToProcess(storageManager, datasourceName)
	if len(datasources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	for _, name := range datasources {
		fmt.Printf("Analyzing %s...\n", name)
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}
		if err := storage.Analyze(); err != nil {
			return fmt.Errorf("analyzing %s: %w", name, err)
		}
	}

	fmt.Println()
	fmt.Println("✓ ANALYZE completed successfully")
	return nil
}

// vacuumDatabase runs VACUUM on all databases
func vacuumDatabase(configPath string, datasourceName string) error {
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

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	if datasourceName != "" {
		fmt.Printf("Running VACUUM on %s...\n", datasourceName)
	} else {
		fmt.Println("Running VACUUM on all databases...")
	}
	fmt.Println("This may take a while for large databases...")
	fmt.Println()

	hasErrors := false

	// Get datasources to process
	datasources := getDatasourcesToProcess(storageManager, datasourceName)
	if len(datasources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	for _, name := range datasources {
		fmt.Printf("Vacuuming %s... ", name)

		// Get the storage and run vacuum
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			fmt.Printf("✗ FAILED - %v\n", err)
			hasErrors = true
			continue
		}

		err = storage.Vacuum()
		if err != nil {
			fmt.Printf("✗ FAILED - %v\n", err)
			hasErrors = true
		} else {
			fmt.Printf("✓ OK\n")
		}
	}

	fmt.Println()
	if hasErrors {
		return fmt.Errorf("VACUUM failed for one or more databases")
	}

	fmt.Println("All databases vacuumed successfully")
	return nil
}

// checkpointDatabase runs WAL checkpoint on all databases
func checkpointDatabase(configPath string, datasourceName string) error {
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

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	if datasourceName != "" {
		fmt.Printf("Running WAL checkpoint on %s...\n", datasourceName)
	} else {
		fmt.Println("Running WAL checkpoint on all databases...")
	}
	fmt.Println()

	// Get datasources to process
	datasources := getDatasourcesToProcess(storageManager, datasourceName)
	if len(datasources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	for _, name := range datasources {
		fmt.Printf("Checkpointing %s...\n", name)
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			return fmt.Errorf("getting storage for %s: %w", name, err)
		}
		if err := storage.WALCheckpoint(); err != nil {
			return fmt.Errorf("checkpointing %s: %w", name, err)
		}
	}

	fmt.Println()
	fmt.Println("✓ WAL checkpoint completed successfully")
	return nil
}

// checkDatabases runs integrity checks on all databases
func checkDatabases(configPath string, datasourceName string, deepFTS bool) error {
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

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	if datasourceName != "" {
		fmt.Printf("Running integrity check on %s", datasourceName)
	} else {
		fmt.Print("Running integrity checks on all databases")
	}
	if !deepFTS {
		fmt.Println(" (quick mode, skipping FTS checks)...")
	} else {
		fmt.Println("...")
	}
	fmt.Println()

	hasErrors := false

	// Get datasources to process
	datasources := getDatasourcesToProcess(storageManager, datasourceName)
	if len(datasources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	for _, name := range datasources {
		fmt.Printf("Checking %s... ", name)

		// Get the storage and run integrity check
		storage, err := storageManager.GetStorage(name)
		if err != nil {
			fmt.Printf("✗ FAILED - %v\n", err)
			hasErrors = true
			continue
		}

		// Run standard integrity check first
		err = storage.IntegrityCheck()
		if err != nil {
			fmt.Printf("✗ FAILED - %v\n", err)
			hasErrors = true
			continue
		}

		// Run deep FTS check if requested
		if deepFTS {
			err = storage.FTSIntegrityCheck()
			if err != nil {
				fmt.Printf("✗ FTS FAILED - %v\n", err)
				hasErrors = true
				continue
			}
		}

		fmt.Printf("✓ OK\n")
	}

	fmt.Println()
	if hasErrors {
		fmt.Println("Some databases failed integrity checks.")
		fmt.Println("To fix FTS index corruption, run: ergs optimize fts-rebuild")
		return fmt.Errorf("integrity check failed for one or more databases")
	}

	fmt.Println("All databases passed integrity checks")
	return nil
}

// rebuildFTS rebuilds FTS5 indexes for all databases
func rebuildFTS(configPath string, datasourceName string, force bool) error {
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

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	// Get datasources to process
	datasources := getDatasourcesToProcess(storageManager, datasourceName)
	if len(datasources) == 0 {
		return fmt.Errorf("no datasources found")
	}

	if datasourceName != "" {
		fmt.Printf("Checking and rebuilding FTS5 index for %s...\n", datasourceName)
	} else {
		fmt.Println("Checking and rebuilding FTS5 indexes for all databases...")
	}
	fmt.Println()

	hasErrors := false
	rebuiltCount := 0
	skippedCount := 0

	for _, name := range datasources {
		// Check FTS integrity first unless --force is used
		if !force {
			fmt.Printf("Checking %s... ", name)

			storage, err := storageManager.GetStorage(name)
			if err != nil {
				fmt.Printf("✗ FAILED - %v\n", err)
				hasErrors = true
				continue
			}

			err = storage.FTSIntegrityCheck()
			if err != nil {
				fmt.Printf("✗ NEEDS REBUILD - %v\n", err)

				// Rebuild immediately
				fmt.Printf("Rebuilding %s... ", name)
				err = storage.FTSRebuild()
				if err != nil {
					fmt.Printf("✗ FAILED - %v\n", err)
					hasErrors = true
				} else {
					fmt.Printf("✓ OK\n")
					rebuiltCount++
				}
			} else {
				fmt.Printf("✓ OK (no rebuild needed)\n")
				skippedCount++
			}
		} else {
			// Force rebuild without checking
			fmt.Printf("Rebuilding %s... ", name)

			storage, err := storageManager.GetStorage(name)
			if err != nil {
				fmt.Printf("✗ FAILED - %v\n", err)
				hasErrors = true
				continue
			}

			err = storage.FTSRebuild()
			if err != nil {
				fmt.Printf("✗ FAILED - %v\n", err)
				hasErrors = true
			} else {
				fmt.Printf("✓ OK\n")
				rebuiltCount++
			}
		}
	}

	fmt.Println()
	if hasErrors {
		return fmt.Errorf("FTS rebuild failed for one or more databases")
	}

	if rebuiltCount > 0 {
		fmt.Printf("Successfully rebuilt %d database(s)\n", rebuiltCount)
	}
	if skippedCount > 0 {
		fmt.Printf("Skipped %d healthy database(s)\n", skippedCount)
	}
	if rebuiltCount == 0 && skippedCount == 0 {
		fmt.Println("No databases processed")
	}

	return nil
}

// getDatasourcesToProcess returns a list of datasources to process based on the datasourceName filter.
// If datasourceName is empty, returns all datasources. Otherwise, returns a single-element slice with the specified datasource.
func getDatasourcesToProcess(storageManager *storage.Manager, datasourceName string) []string {
	allDatasources := storageManager.GetDatasourceNames()

	if datasourceName == "" {
		return allDatasources
	}

	// Check if the specified datasource exists
	for _, name := range allDatasources {
		if name == datasourceName {
			return []string{datasourceName}
		}
	}

	// Datasource not found
	return []string{}
}
