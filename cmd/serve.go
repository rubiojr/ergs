package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/warehouse"
	"github.com/urfave/cli/v3"
)

// ServeCommand creates the serve command
func ServeCommand() *cli.Command {
	return &cli.Command{
		Name:  "serve",
		Usage: "Start the scheduler daemon",
		Action: func(ctx context.Context, c *cli.Command) error {
			return serve(ctx, c.String("config"))
		},
	}
}

// serve starts the scheduler daemon to continuously fetch data
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
