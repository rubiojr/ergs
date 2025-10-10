package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

// StatsCommand creates the stats command
func StatsCommand() *cli.Command {
	return &cli.Command{
		Name:  "stats",
		Usage: "Show statistics",
		Action: func(ctx context.Context, c *cli.Command) error {
			return showStats(c.String("config"))
		},
	}
}

// showStats displays storage statistics
func showStats(configPath string) error {
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

	configuredDatasources := cfg.ListDatasources()
	storageManager, err := storage.NewManager(cfg.StorageDir, configuredDatasources...)
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

	stats, err := storageManager.GetStats()
	if err != nil {
		return fmt.Errorf("getting stats: %w", err)
	}

	formatStats(stats)
	return nil
}
