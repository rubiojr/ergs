package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

// ListCommand creates the list command
func ListCommand() *cli.Command {
	return &cli.Command{
		Name:  "list",
		Usage: "List blocks from a datasource",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:     "datasource",
				Usage:    "Datasource name to list blocks from",
				Required: true,
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum number of blocks to show",
				Value: 20,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return listBlocks(c.String("config"), c.String("datasource"), c.Int("limit"))
		},
	}
}

// listBlocks lists blocks from a specific datasource
func listBlocks(configPath, datasourceName string, limit int) error {
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

	blocks, err := storageManager.SearchBlocks(datasourceName, "", limit)
	if err != nil {
		return fmt.Errorf("listing blocks from %s: %w", datasourceName, err)
	}

	if len(blocks) == 0 {
		fmt.Printf("No blocks found in datasource '%s'\n", datasourceName)
		return nil
	}

	fmt.Printf("=== %s (%d blocks) ===\n\n", datasourceName, len(blocks))

	for i, block := range blocks {
		fmt.Printf("Block %d:\n%s\n", i+1, block.PrettyText())
		if i < len(blocks)-1 {
			fmt.Println()
		}
	}

	return nil
}
