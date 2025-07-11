package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

// SearchCommand creates the search command
func SearchCommand() *cli.Command {
	return &cli.Command{
		Name:  "search",
		Usage: "Search indexed data",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "query",
				Usage: "Search query",
			},
			&cli.StringFlag{
				Name:  "datasource",
				Usage: "Specific datasource to search",
			},
			&cli.IntFlag{
				Name:  "limit",
				Usage: "Maximum number of results",
				Value: 10,
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return searchData(c.String("config"), c.String("query"), c.String("datasource"), c.Int("limit"))
		},
	}
}

// searchData searches for data in the indexed storage
func searchData(configPath, query, datasourceName string, limit int) error {
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

	if datasourceName != "" {
		blocks, err := storageManager.SearchBlocks(datasourceName, query, limit)
		if err != nil {
			return fmt.Errorf("searching %s: %w", datasourceName, err)
		}

		fmt.Printf("Found %d results in %s:\n", len(blocks), datasourceName)
		for i, block := range blocks {
			fmt.Printf("%d. %s\n", i+1, block.PrettyText())
			if i < len(blocks)-1 {
				fmt.Println()
			}
		}
	} else {
		results, err := storageManager.SearchAllDatasources(query, limit)
		if err != nil {
			return fmt.Errorf("searching all datasources: %w", err)
		}

		totalResults := 0
		for datasource, blocks := range results {
			totalResults += len(blocks)
			fmt.Printf("\n=== %s (%d results) ===\n", datasource, len(blocks))
			for i, block := range blocks {
				fmt.Printf("%d. %s\n", i+1, block.PrettyText())
				if i < len(blocks)-1 {
					fmt.Println()
				}
			}
		}

		if totalResults == 0 {
			fmt.Println("No results found")
		} else {
			fmt.Printf("\nTotal: %d results across %d datasources\n", totalResults, len(results))
		}
	}

	return nil
}
