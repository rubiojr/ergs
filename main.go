package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sort"
	"syscall"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/warehouse"
	"github.com/urfave/cli/v3"
)

func main() {
	app := &cli.Command{
		Name:  "ergs",
		Usage: "A generic data fetching and indexing tool",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging",
				Value: false,
			},
			&cli.StringFlag{
				Name:  "config",
				Usage: "Configuration file path",
				Value: config.GetDefaultConfigPath(),
			},
		},
		Commands: []*cli.Command{
			{
				Name:  "init",
				Usage: "Initialize configuration",
				Action: func(ctx context.Context, c *cli.Command) error {
					return initConfig(c.String("config"))
				},
			},
			{
				Name:  "datasource",
				Usage: "Manage datasources",
				Commands: []*cli.Command{
					{
						Name:  "list",
						Usage: "List datasources",
						Action: func(ctx context.Context, c *cli.Command) error {
							return listDatasources(c.String("config"))
						},
					},
					{
						Name:  "remove",
						Usage: "Remove a datasource",
						Flags: []cli.Flag{
							&cli.StringFlag{
								Name:     "name",
								Usage:    "Datasource name",
								Required: true,
							},
						},
						Action: func(ctx context.Context, c *cli.Command) error {
							return removeDatasource(c.String("config"), c.String("name"))
						},
					},
				},
			},
			{
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
			},
			{
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
			},
			{
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
			},
			{
				Name:  "serve",
				Usage: "Start the scheduler daemon",
				Action: func(ctx context.Context, c *cli.Command) error {
					return serve(ctx, c.String("config"))
				},
			},
			{
				Name:  "stats",
				Usage: "Show statistics",
				Action: func(ctx context.Context, c *cli.Command) error {
					return showStats(c.String("config"))
				},
			},
			{
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
			},
			{
				Name:  "version",
				Usage: "Show version information",
				Action: func(ctx context.Context, c *cli.Command) error {
					fmt.Println("ergs version 1.0.0")
					return nil
				},
			},
		},
	}

	if err := app.Run(context.Background(), os.Args); err != nil {
		log.Fatal(err)
	}
}

func initConfig(configPath string) error {
	cfg := config.GetDefaultConfig()
	if err := cfg.SaveTemplateConfig(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}
	fmt.Printf("Configuration initialized at %s\n", configPath)
	return nil
}

func listDatasources(configPath string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	datasources := cfg.ListDatasources()
	if len(datasources) == 0 {
		fmt.Println("No datasources configured")
		return nil
	}

	fmt.Println("Configured datasources:")
	for _, name := range datasources {
		dsType, _, err := cfg.GetDatasourceConfig(name)
		if err != nil {
			fmt.Printf("  %s: error loading config: %v\n", name, err)
			continue
		}
		interval := cfg.GetDatasourceInterval(name)
		fmt.Printf("  %s (%s) - interval: %v\n", name, dsType, interval)
	}

	return nil
}

func removeDatasource(configPath, name string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	cfg.RemoveDatasource(name)

	if err := cfg.SaveConfig(configPath); err != nil {
		return fmt.Errorf("saving config: %w", err)
	}

	fmt.Printf("Removed datasource '%s'\n", name)
	return nil
}

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

func serve(ctx context.Context, configPath string) error {
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
		OptimizeInterval: time.Hour, // Optimize every hour
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer wh.Close()

	datasources := registry.GetAllDatasources()
	log.Printf("Configuring %d datasources:", len(datasources))
	for name, ds := range datasources {
		interval := cfg.GetDatasourceInterval(name)
		log.Printf("  - %s: %v", name, interval)
		if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
			return fmt.Errorf("adding datasource to warehouse: %w", err)
		}
	}

	// Create a cancellable context for the warehouse
	warehouseCtx, warehouseCancel := context.WithCancel(ctx)
	defer warehouseCancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	fmt.Println("Starting warehouse with per-datasource intervals")

	// Start warehouse (non-blocking)
	if err := wh.Start(warehouseCtx); err != nil {
		return fmt.Errorf("starting warehouse: %w", err)
	}

	fmt.Println("Warehouse started. Press Ctrl+C to stop.")

	// Wait for signal
	<-sigCh
	fmt.Println("\nShutting down...")
	warehouseCancel() // Cancel the warehouse context
	wh.Stop()         // Stop the warehouse and wait for completion
	return nil
}

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

func formatStats(stats map[string]interface{}) {
	// Print summary
	fmt.Printf("📊 Storage Statistics\n")
	fmt.Printf("═══════════════════════\n\n")

	totalBlocks, _ := stats["total_blocks"].(int)
	totalDatasources, _ := stats["total_datasources"].(int)

	fmt.Printf("Total blocks: %s\n", formatNumber(totalBlocks))
	fmt.Printf("Total datasources: %d\n\n", totalDatasources)

	if totalDatasources == 0 {
		fmt.Printf("No datasources configured yet.\n")
		return
	}

	// Get datasource names and sort them
	var datasourceNames []string
	for key := range stats {
		if key != "total_blocks" && key != "total_datasources" {
			datasourceNames = append(datasourceNames, key)
		}
	}
	sort.Strings(datasourceNames)

	// Print each datasource
	fmt.Printf("Datasource Details:\n")
	fmt.Printf("───────────────────\n")

	for i, name := range datasourceNames {
		if i > 0 {
			fmt.Printf("\n")
		}

		dsStats, ok := stats[name].(map[string]interface{})
		if !ok {
			fmt.Printf("❌ %s: No data available\n", name)
			continue
		}

		fmt.Printf("📁 %s\n", name)

		if totalBlocks, ok := dsStats["total_blocks"].(int); ok {
			fmt.Printf("   Blocks: %s", formatNumber(totalBlocks))

			// Calculate percentage
			if totalBlocks > 0 {
				percentage := float64(totalBlocks) / float64(stats["total_blocks"].(int)) * 100
				fmt.Printf(" (%.1f%%)", percentage)
			}
			fmt.Printf("\n")
		}

		if oldestBlock, ok := dsStats["oldest_block"].(time.Time); ok {
			fmt.Printf("   Oldest: %s\n", formatTime(oldestBlock))
		}

		if newestBlock, ok := dsStats["newest_block"].(time.Time); ok {
			fmt.Printf("   Newest: %s\n", formatTime(newestBlock))

			// Calculate time span
			if oldestBlock, ok := dsStats["oldest_block"].(time.Time); ok {
				duration := newestBlock.Sub(oldestBlock)
				fmt.Printf("   Span:   %s\n", formatDuration(duration))
			}
		}
	}
}

func formatNumber(n int) string {
	if n < 1000 {
		return fmt.Sprintf("%d", n)
	} else if n < 1000000 {
		return fmt.Sprintf("%.1fK", float64(n)/1000)
	} else {
		return fmt.Sprintf("%.1fM", float64(n)/1000000)
	}
}

func formatTime(t time.Time) string {
	now := time.Now()
	diff := now.Sub(t)

	// If it's within the last day, show relative time
	if diff < 24*time.Hour {
		if diff < time.Hour {
			minutes := int(diff.Minutes())
			if minutes < 1 {
				return "just now"
			}
			return fmt.Sprintf("%d minutes ago", minutes)
		}
		hours := int(diff.Hours())
		return fmt.Sprintf("%d hours ago", hours)
	}

	// If it's within the last week, show days ago
	if diff < 7*24*time.Hour {
		days := int(diff.Hours() / 24)
		return fmt.Sprintf("%d days ago", days)
	}

	// Otherwise show the date
	if t.Year() == now.Year() {
		return t.Format("Jan 2, 15:04")
	}
	return t.Format("Jan 2, 2006")
}

func formatDuration(d time.Duration) string {
	if d < time.Hour {
		return fmt.Sprintf("%d minutes", int(d.Minutes()))
	} else if d < 24*time.Hour {
		return fmt.Sprintf("%.1f hours", d.Hours())
	} else if d < 30*24*time.Hour {
		return fmt.Sprintf("%.1f days", d.Hours()/24)
	} else if d < 365*24*time.Hour {
		return fmt.Sprintf("%.1f months", d.Hours()/(24*30))
	} else {
		return fmt.Sprintf("%.1f years", d.Hours()/(24*365))
	}
}

func listBlocks(configPath, datasourceName string, limit int) error {
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

func createDatasourcesFromConfig(registry *core.Registry, cfg *config.Config) error {
	for name := range cfg.Datasources {
		dsType, dsConfigRaw, err := cfg.GetDatasourceConfig(name)
		if err != nil {
			return fmt.Errorf("getting config for datasource %s: %w", name, err)
		}

		// Create datasource with empty config first
		if err := registry.CreateDatasource(name, dsType, nil); err != nil {
			return fmt.Errorf("creating datasource %s: %w", name, err)
		}

		// Get the datasource and configure it
		datasources := registry.GetAllDatasources()
		ds, exists := datasources[name]
		if !exists {
			return fmt.Errorf("datasource %s not found after creation", name)
		}

		// Convert the raw config to the proper type using the datasource's ConfigType
		dsConfig, err := convertRawConfigToType(ds, dsConfigRaw)
		if err != nil {
			return fmt.Errorf("converting config for datasource %s: %w", name, err)
		}

		// Set the config on the datasource
		if err := ds.SetConfig(dsConfig); err != nil {
			return fmt.Errorf("setting config for datasource %s: %w", name, err)
		}
	}

	return nil
}

func convertRawConfigToType(ds core.Datasource, rawConfig interface{}) (interface{}, error) {
	// Get the expected config type from the datasource
	configType := ds.ConfigType()

	if rawConfig == nil {
		// Return the default config type
		return configType, nil
	}

	// Marshal and unmarshal to convert between types
	configData, err := toml.Marshal(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("marshaling config data: %w", err)
	}

	if err := toml.Unmarshal(configData, configType); err != nil {
		return nil, fmt.Errorf("unmarshaling datasource config: %w", err)
	}

	return configType, nil
}

func initializeDatasourceStorage(registry *core.Registry, storageManager *storage.Manager) error {
	datasources := registry.GetAllDatasources()
	for name, ds := range datasources {
		schema := ds.Schema()
		if err := storageManager.InitializeDatasourceStorage(name, schema); err != nil {
			return fmt.Errorf("initializing storage for %s: %w", name, err)
		}

		// Register the block prototype for this datasource
		storageManager.RegisterBlockPrototype(name, ds.BlockPrototype())
	}

	return nil
}

func optimizeDatabase(configPath string, analyze, vacuum, checkpoint bool) error {
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
