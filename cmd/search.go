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

	// Use search service for consistent behavior
	searchService := storageManager.GetSearchService()

	// Build search parameters
	params := storage.SearchParams{
		Query: query,
		Page:  1,
		Limit: limit,
	}

	// Add datasource filter if specified
	if datasourceName != "" {
		params.DatasourceFilters = []string{datasourceName}
	}

	results, err := searchService.Search(params)
	if err != nil {
		return fmt.Errorf("executing search: %w", err)
	}

	// Display results
	if len(results.Results) == 0 {
		fmt.Println("No results found")
		return nil
	}

	totalResults := 0
	for datasource, blocks := range results.Results {
		totalResults += len(blocks)
		if datasourceName != "" {
			fmt.Printf("Found %d results in %s:\n", len(blocks), datasource)
		} else {
			fmt.Printf("\n=== %s (%d results) ===\n", datasource, len(blocks))
		}

		for i, block := range blocks {
			fmt.Printf("%d. %s\n", i+1, block.PrettyText())
			if i < len(blocks)-1 {
				fmt.Println()
			}
		}
	}

	if datasourceName == "" && totalResults > 0 {
		fmt.Printf("\nTotal: %d results across %d datasources\n", totalResults, len(results.Results))
	}

	return nil
}
