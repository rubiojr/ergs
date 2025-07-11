package cmd

import (
	"context"
	"fmt"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/warehouse"
	"github.com/urfave/cli/v3"
)

// FetchCommand creates the fetch command
func FetchCommand() *cli.Command {
	return &cli.Command{
		Name:  "fetch",
		Usage: "Fetch data from all datasources",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "stream",
				Usage: "Stream blocks to stdout as they are received",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "datasource",
				Usage: "Specific datasource to fetch from",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return fetchData(ctx, c.String("config"), c.Bool("stream"), c.String("datasource"))
		},
	}
}

// fetchData fetches data from configured datasources
func fetchData(ctx context.Context, configPath string, stream bool, datasourceName string) error {
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

	warehouseConfig := warehouse.Config{
		OptimizeInterval: 0, // No optimization for one-time fetch
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer wh.Close()

	datasources := registry.GetAllDatasources()

	// If a specific datasource is requested, filter to only that one
	if datasourceName != "" {
		if ds, exists := datasources[datasourceName]; exists {
			datasources = map[string]core.Datasource{datasourceName: ds}
		} else {
			return fmt.Errorf("datasource '%s' not found", datasourceName)
		}
	}

	for name, ds := range datasources {
		interval := cfg.GetDatasourceInterval(name)
		if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
			return fmt.Errorf("adding datasource to warehouse: %w", err)
		}
	}

	if stream {
		if datasourceName != "" {
			fmt.Printf("Streaming blocks from datasource '%s' as they are received...\n", datasourceName)
		} else {
			fmt.Println("Streaming blocks as they are received...")
		}
		if err := wh.FetchOnceWithStreaming(ctx, func(block core.Block) {
			fmt.Printf("%s\n\n", block.PrettyText())
		}); err != nil {
			return fmt.Errorf("fetching data: %w", err)
		}
	} else {
		if datasourceName != "" {
			fmt.Printf("Fetching data from datasource '%s'...\n", datasourceName)
		} else {
			fmt.Println("Fetching data from all datasources...")
		}
		if err := wh.FetchOnce(ctx); err != nil {
			return fmt.Errorf("fetching data: %w", err)
		}
	}

	fmt.Println("Fetch completed successfully")
	return nil
}
