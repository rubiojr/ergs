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
		Usage: "Optimize database performance",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "analyze",
				Usage: "Run ANALYZE to update query planner statistics",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "vacuum",
				Usage: "Run VACUUM to defragment database",
				Value: false,
			},
			&cli.BoolFlag{
				Name:  "checkpoint",
				Usage: "Run WAL checkpoint to flush changes",
				Value: false,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return optimizeDatabase(c.String("config"), c.Bool("analyze"), c.Bool("vacuum"), c.Bool("checkpoint"))
		},
	}
}

// optimizeDatabase performs database optimization operations
func optimizeDatabase(configPath string, analyze, vacuum, checkpoint bool) error {
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

	storageManager := storage.NewManager(cfg.StorageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	fmt.Println("Optimizing databases...")

	// Always run basic optimization
	if err := storageManager.OptimizeAll(); err != nil {
		return fmt.Errorf("optimizing databases: %w", err)
	}
	fmt.Println("✓ Basic optimization completed")

	if analyze {
		fmt.Println("Running ANALYZE...")
		if err := storageManager.AnalyzeAll(); err != nil {
			return fmt.Errorf("analyzing databases: %w", err)
		}
		fmt.Println("✓ ANALYZE completed")
	}

	if checkpoint {
		fmt.Println("Running WAL checkpoint...")
		if err := storageManager.WALCheckpointAll(); err != nil {
			return fmt.Errorf("WAL checkpoint: %w", err)
		}
		fmt.Println("✓ WAL checkpoint completed")
	}

	if vacuum {
		fmt.Println("Running VACUUM (this may take a while)...")
		// VACUUM is not available in the manager, we'd need to implement it per storage
		fmt.Println("⚠ VACUUM not yet implemented for all storages")
	}

	fmt.Println("Database optimization completed successfully")
	return nil
}
