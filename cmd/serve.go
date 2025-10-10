package cmd

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/fsnotify/fsnotify"
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

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour, // Optimize every hour
		EventSocketPath:  cfg.EventSocketPath,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			fmt.Printf("Warning: failed to close warehouse: %v\n", err)
		}
	}()

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

	// Signal handling - now includes SIGHUP for reload
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)

	fmt.Println("Starting warehouse with per-datasource intervals")

	// Start warehouse (non-blocking)
	if err := wh.Start(warehouseCtx); err != nil {
		return fmt.Errorf("starting warehouse: %w", err)
	}

	fmt.Println("Warehouse started. Press Ctrl+C to stop, send SIGHUP to reload, or modify config file for automatic reload.")

	// Configuration reload state
	var cfgMutex sync.RWMutex
	currentConfig := cfg

	// Set up filesystem watcher for config file
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		log.Printf("Warning: failed to create config file watcher: %v", err)
	} else {
		defer func() {
			if err := watcher.Close(); err != nil {
				log.Printf("Warning: failed to close config file watcher: %v", err)
			}
		}()

		// Add config file to watcher
		if err := watcher.Add(configPath); err != nil {
			log.Printf("Warning: failed to watch config file %s: %v", configPath, err)
		} else {
			log.Printf("Watching config file for changes: %s", configPath)
		}
	}

	// Main event handling loop
	for {
		select {
		case sig := <-sigCh:
			switch sig {
			case syscall.SIGHUP:
				log.Println("Received SIGHUP, reloading configuration...")
				if err := reloadConfiguration(configPath, registry, wh, &cfgMutex, &currentConfig); err != nil {
					log.Printf("Failed to reload configuration: %v", err)
				} else {
					log.Println("Configuration reloaded successfully")
				}
			case syscall.SIGINT, syscall.SIGTERM:
				fmt.Println("\nShutting down...")
				warehouseCancel() // Cancel the warehouse context
				wh.Stop()         // Stop the warehouse and wait for completion
				return nil
			}
		case event, ok := <-watcher.Events:
			if !ok {
				continue
			}
			// React to write, create, rename, and remove events (editors often use atomic writes)
			if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
				log.Printf("Config file changed: %s (event: %s), reloading configuration...", event.Name, event.Op.String())

				// For rename/remove events, we need to re-add the file to the watcher since it was replaced
				if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					// Small delay to ensure the new file is fully written
					time.Sleep(200 * time.Millisecond)

					// Check if file was actually replaced (atomic write) or just removed
					if _, err := os.Stat(configPath); os.IsNotExist(err) {
						log.Printf("Config file was removed and not replaced, skipping reload")
						continue
					}

					// Re-add the config file to watcher in case it was replaced
					if err := watcher.Add(configPath); err != nil {
						log.Printf("Warning: failed to re-add config file to watcher after rename/remove: %v", err)
					}
				} else {
					// Add a small delay to ensure file write is complete
					time.Sleep(100 * time.Millisecond)
				}

				if err := reloadConfiguration(configPath, registry, wh, &cfgMutex, &currentConfig); err != nil {
					log.Printf("Failed to reload configuration after file change: %v", err)
				} else {
					log.Println("Configuration reloaded successfully after file change")
				}
			}
		case err, ok := <-watcher.Errors:
			if !ok {
				continue
			}
			log.Printf("Config file watcher error: %v", err)
		}
	}
}

// reloadConfiguration handles the configuration reload process
func reloadConfiguration(configPath string, registry *core.Registry, wh *warehouse.Warehouse, cfgMutex *sync.RWMutex, currentConfig **config.Config) error {
	cfgMutex.Lock()
	defer cfgMutex.Unlock()

	// Load new configuration
	newCfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading new config: %w", err)
	}

	oldCfg := *currentConfig

	// Remove all existing datasources
	oldDatasources := oldCfg.ListDatasources()
	for _, name := range oldDatasources {
		log.Printf("Removing datasource: %s", name)
		if err := removeDatasourceFromWarehouse(wh, registry, name); err != nil {
			log.Printf("Warning: failed to remove datasource %s: %v", name, err)
		}
	}

	// Add all datasources from new configuration
	newDatasources := newCfg.ListDatasources()
	for _, name := range newDatasources {
		log.Printf("Adding datasource: %s", name)
		if err := addDatasourceToWarehouse(wh, registry, newCfg, name); err != nil {
			return fmt.Errorf("adding datasource %s: %w", name, err)
		}
	}

	// Update current config
	*currentConfig = newCfg

	log.Printf("Configuration reload complete: removed %d datasources, added %d datasources",
		len(oldDatasources), len(newDatasources))

	return nil
}

// removeDatasourceFromWarehouse removes a datasource from the warehouse and registry
func removeDatasourceFromWarehouse(wh *warehouse.Warehouse, registry *core.Registry, name string) error {
	// Remove from warehouse first
	if err := wh.RemoveDatasource(name); err != nil {
		return fmt.Errorf("removing datasource from warehouse: %w", err)
	}

	// Remove from registry
	if err := registry.RemoveDatasource(name); err != nil {
		return fmt.Errorf("removing datasource from registry: %w", err)
	}

	return nil
}

// addDatasourceToWarehouse adds a new datasource to the warehouse and registry
func addDatasourceToWarehouse(wh *warehouse.Warehouse, registry *core.Registry, cfg *config.Config, name string) error {
	dsType, dsConfigRaw, err := cfg.GetDatasourceConfig(name)
	if err != nil {
		return fmt.Errorf("getting config for datasource %s: %w", name, err)
	}

	// Create datasource in registry
	if err := registry.CreateDatasource(name, dsType, nil); err != nil {
		return fmt.Errorf("creating datasource %s: %w", name, err)
	}

	// Get the datasource and configure it
	datasources := registry.GetAllDatasources()
	ds, exists := datasources[name]
	if !exists {
		return fmt.Errorf("datasource %s not found after creation", name)
	}

	// Convert and set config
	dsConfig, err := convertRawConfigToType(ds, dsConfigRaw)
	if err != nil {
		return fmt.Errorf("converting config for datasource %s: %w", name, err)
	}

	if err := ds.SetConfig(dsConfig); err != nil {
		return fmt.Errorf("setting config for datasource %s: %w", name, err)
	}

	// Add to warehouse
	interval := cfg.GetDatasourceInterval(name)
	if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
		return fmt.Errorf("adding datasource %s to warehouse: %w", name, err)
	}

	return nil
}
