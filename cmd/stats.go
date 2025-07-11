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
	defer registry.Close()

	storageManager := storage.NewManager(cfg.StorageDir)
	defer storageManager.Close()

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
