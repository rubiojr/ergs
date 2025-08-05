package integration_tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/pelletier/go-toml/v2"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/warehouse"

	// Import only timestamp datasource (no network required)
	"github.com/fsnotify/fsnotify"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
)

func TestSIGHUPReload(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create initial configuration with one datasource
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasource
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Add datasource to warehouse
	datasources := registry.GetAllDatasources()
	for name, ds := range datasources {
		interval := initialConfig.GetDatasourceInterval(name)
		if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
			t.Fatalf("Failed to add datasource to warehouse: %v", err)
		}
	}

	// Start warehouse in background
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := wh.Start(ctx); err != nil {
		t.Fatalf("Failed to start warehouse: %v", err)
	}
	defer wh.Stop()

	// Verify initial state
	if !wh.IsRunning() {
		t.Fatal("Warehouse should be running")
	}

	initialDatasources := registry.ListDatasources()
	if len(initialDatasources) != 1 {
		t.Fatalf("Expected 1 datasource, got %d", len(initialDatasources))
	}
	if initialDatasources[0] != "test-timestamp" {
		t.Fatalf("Expected datasource 'test-timestamp', got '%s'", initialDatasources[0])
	}

	// Create updated configuration with different datasources
	updatedConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp-new": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 120, // Different interval
				},
			},
			"test-timestamp-2": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 30,
				},
			},
		},
	}

	// Save updated config
	if err := updatedConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save updated config: %v", err)
	}

	// Test the reload function directly (simulating SIGHUP handling)
	var cfgMutex, currentConfigPtr = getCurrentConfigForTest(initialConfig)
	if err := reloadConfigurationForTest(configPath, registry, wh, cfgMutex, currentConfigPtr); err != nil {
		t.Fatalf("Failed to reload configuration: %v", err)
	}

	// Verify the configuration was reloaded
	newDatasources := registry.ListDatasources()
	if len(newDatasources) != 2 {
		t.Fatalf("Expected 2 datasources after reload, got %d", len(newDatasources))
	}

	expectedDatasources := map[string]bool{
		"test-timestamp-new": false,
		"test-timestamp-2":   false,
	}

	for _, name := range newDatasources {
		if _, exists := expectedDatasources[name]; exists {
			expectedDatasources[name] = true
		} else {
			t.Fatalf("Unexpected datasource after reload: %s", name)
		}
	}

	for name, found := range expectedDatasources {
		if !found {
			t.Fatalf("Expected datasource '%s' not found after reload", name)
		}
	}

	// Test that old datasource was removed
	_, err := registry.GetDatasource("test-timestamp")
	if err == nil {
		t.Fatal("Old datasource 'test-timestamp' should have been removed")
	}

	// Test that new datasources exist and are properly configured
	for name := range expectedDatasources {
		ds, err := registry.GetDatasource(name)
		if err != nil {
			t.Fatalf("Failed to get datasource '%s': %v", name, err)
		}

		if ds.Type() != "timestamp" {
			t.Fatalf("Expected datasource '%s' to be of type 'timestamp', got '%s'", name, ds.Type())
		}
	}
}

func TestReloadWithInvalidConfig(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create valid initial configuration
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasource
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Write invalid config file
	invalidConfigContent := `
[invalid toml syntax
datasources = broken
`
	if err := os.WriteFile(configPath, []byte(invalidConfigContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Test that reload fails gracefully with invalid config
	var cfgMutex, currentConfigPtr = getCurrentConfigForTest(initialConfig)
	err := reloadConfigurationForTest(configPath, registry, wh, cfgMutex, currentConfigPtr)
	if err == nil {
		t.Fatal("Expected reload to fail with invalid config")
	}

	// Verify that the old configuration is still intact
	datasources := registry.ListDatasources()
	if len(datasources) != 1 || datasources[0] != "test-timestamp" {
		t.Fatal("Original datasources should still be intact after failed reload")
	}
}

func TestReloadEmptyConfig(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create initial configuration with datasources
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp-1": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
			"test-timestamp-2": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 120,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasources
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Verify initial state
	initialDatasources := registry.ListDatasources()
	if len(initialDatasources) != 2 {
		t.Fatalf("Expected 2 initial datasources, got %d", len(initialDatasources))
	}

	// Create empty configuration (no datasources)
	emptyConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources:   make(map[string]config.DatasourceInfo),
	}

	// Save empty config
	if err := emptyConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save empty config: %v", err)
	}

	// Test reload with empty config
	var cfgMutex, currentConfigPtr = getCurrentConfigForTest(initialConfig)
	if err := reloadConfigurationForTest(configPath, registry, wh, cfgMutex, currentConfigPtr); err != nil {
		t.Fatalf("Failed to reload empty configuration: %v", err)
	}

	// Verify all datasources were removed
	newDatasources := registry.ListDatasources()
	if len(newDatasources) != 0 {
		t.Fatalf("Expected 0 datasources after empty reload, got %d: %v", len(newDatasources), newDatasources)
	}
}

// Helper functions for testing

func getCurrentConfigForTest(cfg *config.Config) (*sync.RWMutex, **config.Config) {
	var cfgMutex sync.RWMutex
	currentConfig := cfg
	return &cfgMutex, &currentConfig
}

// These functions are copies of the internal functions for testing
// In a real implementation, you might want to make them package-visible for testing

func createDatasourcesFromConfig(registry *core.Registry, cfg *config.Config) error {
	for name := range cfg.Datasources {
		dsType, dsConfigRaw, err := cfg.GetDatasourceConfig(name)
		if err != nil {
			return fmt.Errorf("getting config for datasource %s: %w", name, err)
		}

		if err := registry.CreateDatasource(name, dsType, nil); err != nil {
			return fmt.Errorf("creating datasource %s: %w", name, err)
		}

		datasources := registry.GetAllDatasources()
		ds, exists := datasources[name]
		if !exists {
			return fmt.Errorf("datasource %s not found after creation", name)
		}

		dsConfig, err := convertRawConfigToType(ds, dsConfigRaw)
		if err != nil {
			return fmt.Errorf("converting config for datasource %s: %w", name, err)
		}

		if err := ds.SetConfig(dsConfig); err != nil {
			return fmt.Errorf("setting config for datasource %s: %w", name, err)
		}
	}

	return nil
}

func convertRawConfigToType(ds core.Datasource, rawConfig interface{}) (interface{}, error) {
	configType := ds.ConfigType()

	if rawConfig == nil {
		return configType, nil
	}

	configData, err := toml.Marshal(rawConfig)
	if err != nil {
		return nil, fmt.Errorf("marshaling config data: %w", err)
	}

	if err := toml.Unmarshal(configData, configType); err != nil {
		return nil, fmt.Errorf("unmarshaling datasource config: %w", err)
	}

	return configType, nil
}

func reloadConfigurationForTest(configPath string, registry *core.Registry, wh *warehouse.Warehouse, cfgMutex *sync.RWMutex, currentConfig **config.Config) error {
	cfgMutex.Lock()
	defer cfgMutex.Unlock()

	newCfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading new config: %w", err)
	}

	oldCfg := *currentConfig

	// Remove all existing datasources
	oldDatasources := oldCfg.ListDatasources()
	for _, name := range oldDatasources {
		if err := removeDatasourceFromWarehouseForTest(wh, registry, name); err != nil {
			return fmt.Errorf("failed to remove datasource %s: %w", name, err)
		}
	}

	// Add all datasources from new configuration
	newDatasources := newCfg.ListDatasources()
	for _, name := range newDatasources {
		if err := addDatasourceToWarehouseForTest(wh, registry, newCfg, name); err != nil {
			return fmt.Errorf("adding datasource %s: %w", name, err)
		}
	}

	*currentConfig = newCfg
	return nil
}

func removeDatasourceFromWarehouseForTest(wh *warehouse.Warehouse, registry *core.Registry, name string) error {
	if err := wh.RemoveDatasource(name); err != nil {
		return fmt.Errorf("removing datasource from warehouse: %w", err)
	}

	if err := registry.RemoveDatasource(name); err != nil {
		return fmt.Errorf("removing datasource from registry: %w", err)
	}

	return nil
}

func TestConfigFileWatching(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create initial configuration with one datasource
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasource
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Add datasource to warehouse
	datasources := registry.GetAllDatasources()
	for name, ds := range datasources {
		interval := initialConfig.GetDatasourceInterval(name)
		if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
			t.Fatalf("Failed to add datasource to warehouse: %v", err)
		}
	}

	// Verify initial state
	initialDatasources := registry.ListDatasources()
	if len(initialDatasources) != 1 {
		t.Fatalf("Expected 1 datasource, got %d", len(initialDatasources))
	}
	if initialDatasources[0] != "test-timestamp" {
		t.Fatalf("Expected datasource 'test-timestamp', got '%s'", initialDatasources[0])
	}

	// Set up file watcher (simulating what serve.go does)
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Failed to close watcher: %v", err)
		}
	}()

	if err := watcher.Add(configPath); err != nil {
		t.Fatalf("Failed to add config file to watcher: %v", err)
	}

	// Configuration reload state
	var cfgMutex sync.RWMutex
	currentConfig := initialConfig

	// Start watching in background
	watcherDone := make(chan bool)
	reloadTriggered := make(chan bool, 1)

	go func() {
		defer close(watcherDone)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					// Add small delay like in serve.go
					time.Sleep(100 * time.Millisecond)
					if err := reloadConfigurationForTest(configPath, registry, wh, &cfgMutex, &currentConfig); err != nil {
						t.Errorf("Failed to reload configuration: %v", err)
					}
					select {
					case reloadTriggered <- true:
					default:
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				t.Errorf("Watcher error: %v", err)
			case <-time.After(5 * time.Second):
				// Timeout to prevent test hanging
				return
			}
		}
	}()

	// Create updated configuration
	updatedConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp-modified": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 120,
				},
			},
		},
	}

	// Modify the config file to trigger the watcher
	if err := updatedConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save updated config: %v", err)
	}

	// Wait for reload to be triggered
	select {
	case <-reloadTriggered:
		// Success - reload was triggered
	case <-time.After(2 * time.Second):
		t.Fatal("Config file change did not trigger reload within timeout")
	}

	// Give some time for reload to complete
	time.Sleep(200 * time.Millisecond)

	// Verify the configuration was reloaded
	newDatasources := registry.ListDatasources()
	if len(newDatasources) != 1 {
		t.Fatalf("Expected 1 datasource after file change, got %d", len(newDatasources))
	}

	if newDatasources[0] != "test-timestamp-modified" {
		t.Fatalf("Expected datasource 'test-timestamp-modified', got '%s'", newDatasources[0])
	}

	// Test that old datasource was removed
	_, err = registry.GetDatasource("test-timestamp")
	if err == nil {
		t.Fatal("Old datasource 'test-timestamp' should have been removed")
	}

	// Test that new datasource exists and is properly configured
	ds, err := registry.GetDatasource("test-timestamp-modified")
	if err != nil {
		t.Fatalf("Failed to get new datasource: %v", err)
	}

	if ds.Type() != "timestamp" {
		t.Fatalf("Expected new datasource to be of type 'timestamp', got '%s'", ds.Type())
	}

	// Stop watcher
	if err := watcher.Close(); err != nil {
		t.Errorf("Failed to close watcher: %v", err)
	}
	<-watcherDone
}

func TestConfigFileWatchingWithInvalidFile(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create valid initial configuration
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasource
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Failed to close watcher: %v", err)
		}
	}()

	if err := watcher.Add(configPath); err != nil {
		t.Fatalf("Failed to add config file to watcher: %v", err)
	}

	// Configuration reload state
	var cfgMutex sync.RWMutex
	currentConfig := initialConfig

	// Track reload attempts
	reloadAttempts := 0
	watcherDone := make(chan bool)

	go func() {
		defer close(watcherDone)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) {
					time.Sleep(100 * time.Millisecond)
					reloadAttempts++
					// This should fail due to invalid config
					_ = reloadConfigurationForTest(configPath, registry, wh, &cfgMutex, &currentConfig)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				t.Errorf("Watcher error: %v", err)
			case <-time.After(3 * time.Second):
				return
			}
		}
	}()

	// Write invalid config file
	invalidConfigContent := `
[invalid toml syntax
datasources = broken
`
	if err := os.WriteFile(configPath, []byte(invalidConfigContent), 0644); err != nil {
		t.Fatalf("Failed to write invalid config: %v", err)
	}

	// Wait for watcher to process the change
	time.Sleep(500 * time.Millisecond)

	// Stop watcher
	if err := watcher.Close(); err != nil {
		t.Errorf("Failed to close watcher: %v", err)
	}
	<-watcherDone

	// Verify reload was attempted but failed gracefully
	if reloadAttempts == 0 {
		t.Fatal("Expected at least one reload attempt after file change")
	}

	// Verify that the old configuration is still intact
	datasources := registry.ListDatasources()
	if len(datasources) != 1 || datasources[0] != "test-timestamp" {
		t.Fatal("Original datasources should still be intact after failed reload")
	}
}

func TestConfigFileWatchingWithAtomicWrites(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create initial configuration
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasource
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Failed to close watcher: %v", err)
		}
	}()

	if err := watcher.Add(configPath); err != nil {
		t.Fatalf("Failed to add config file to watcher: %v", err)
	}

	// Configuration reload state
	var cfgMutex sync.RWMutex
	currentConfig := initialConfig

	// Track reload events
	reloadTriggered := make(chan bool, 1)
	watcherDone := make(chan bool)

	go func() {
		defer close(watcherDone)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Handle write, create, rename, and remove events (like in serve.go)
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					// For rename/remove events, re-add the file to watcher
					if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
						time.Sleep(200 * time.Millisecond)

						// Check if file was actually replaced (atomic write) or just removed
						if _, err := os.Stat(configPath); os.IsNotExist(err) {
							continue
						}

						if err := watcher.Add(configPath); err != nil {
							t.Errorf("Failed to re-add file to watcher: %v", err)
						}
					} else {
						time.Sleep(100 * time.Millisecond)
					}

					if err := reloadConfigurationForTest(configPath, registry, wh, &cfgMutex, &currentConfig); err != nil {
						t.Errorf("Failed to reload configuration: %v", err)
					}
					select {
					case reloadTriggered <- true:
					default:
					}
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				t.Errorf("Watcher error: %v", err)
			case <-time.After(5 * time.Second):
				return
			}
		}
	}()

	// Create updated configuration
	updatedConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp-atomic": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 90,
				},
			},
		},
	}

	// Simulate atomic write behavior (like vim, nano, VS Code)
	// 1. Write to temporary file
	tempFile := configPath + ".tmp"
	if err := updatedConfig.SaveConfig(tempFile); err != nil {
		t.Fatalf("Failed to save temp config: %v", err)
	}

	// 2. Atomically rename temp file over original (this triggers fsnotify.Rename)
	if err := os.Rename(tempFile, configPath); err != nil {
		t.Fatalf("Failed to rename temp config: %v", err)
	}

	// Wait for reload to be triggered
	select {
	case <-reloadTriggered:
		// Success - reload was triggered by rename
	case <-time.After(3 * time.Second):
		t.Fatal("Atomic file write (rename) did not trigger reload within timeout")
	}

	// Give some time for reload to complete
	time.Sleep(200 * time.Millisecond)

	// Verify the configuration was reloaded
	newDatasources := registry.ListDatasources()
	if len(newDatasources) != 1 {
		t.Fatalf("Expected 1 datasource after atomic write, got %d", len(newDatasources))
	}

	if newDatasources[0] != "test-timestamp-atomic" {
		t.Fatalf("Expected datasource 'test-timestamp-atomic', got '%s'", newDatasources[0])
	}

	// Test that old datasource was removed
	_, err = registry.GetDatasource("test-timestamp")
	if err == nil {
		t.Fatal("Old datasource 'test-timestamp' should have been removed after atomic write")
	}

	// Stop watcher
	if err := watcher.Close(); err != nil {
		t.Errorf("Failed to close watcher: %v", err)
	}
	<-watcherDone
}

func TestConfigFileRemovalWithoutReplacement(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "config.toml")
	storageDir := filepath.Join(tempDir, "storage")

	// Create initial configuration
	initialConfig := &config.Config{
		StorageDir:    storageDir,
		FetchInterval: config.Duration{Duration: 30 * time.Minute},
		Datasources: map[string]config.DatasourceInfo{
			"test-timestamp": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}

	// Save initial config
	if err := initialConfig.SaveConfig(configPath); err != nil {
		t.Fatalf("Failed to save initial config: %v", err)
	}

	// Ensure storage directory exists
	if err := os.MkdirAll(storageDir, 0755); err != nil {
		t.Fatalf("Failed to create storage directory: %v", err)
	}

	// Create registry and warehouse
	registry := core.GetGlobalRegistry()
	storageManager := storage.NewManagerWithoutMigrationCheck(storageDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: time.Hour,
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Errorf("Failed to close warehouse: %v", err)
		}
	}()

	// Create initial datasource
	if err := createDatasourcesFromConfig(registry, initialConfig); err != nil {
		t.Fatalf("Failed to create initial datasources: %v", err)
	}

	// Set up file watcher
	watcher, err := fsnotify.NewWatcher()
	if err != nil {
		t.Fatalf("Failed to create watcher: %v", err)
	}
	defer func() {
		if err := watcher.Close(); err != nil {
			t.Errorf("Failed to close watcher: %v", err)
		}
	}()

	if err := watcher.Add(configPath); err != nil {
		t.Fatalf("Failed to add config file to watcher: %v", err)
	}

	// Configuration reload state
	var cfgMutex sync.RWMutex
	currentConfig := initialConfig

	// Track events
	reloadAttempted := false
	watcherDone := make(chan bool)

	go func() {
		defer close(watcherDone)
		for {
			select {
			case event, ok := <-watcher.Events:
				if !ok {
					return
				}
				// Handle write, create, rename, and remove events (like in serve.go)
				if event.Has(fsnotify.Write) || event.Has(fsnotify.Create) || event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
					// For rename/remove events, check if file still exists
					if event.Has(fsnotify.Rename) || event.Has(fsnotify.Remove) {
						time.Sleep(200 * time.Millisecond)
						// Check if file was actually replaced (atomic write) or just removed
						if _, err := os.Stat(configPath); os.IsNotExist(err) {
							continue
						}
						_ = watcher.Add(configPath) // Re-add file to watcher
					} else {
						time.Sleep(100 * time.Millisecond)
					}

					reloadAttempted = true
					_ = reloadConfigurationForTest(configPath, registry, wh, &cfgMutex, &currentConfig)
				}
			case err, ok := <-watcher.Errors:
				if !ok {
					return
				}
				t.Errorf("Watcher error: %v", err)
			case <-time.After(3 * time.Second):
				return
			}
		}
	}()

	// Remove the config file (without replacement)
	if err := os.Remove(configPath); err != nil {
		t.Fatalf("Failed to remove config file: %v", err)
	}

	// Wait for watcher to process the removal
	time.Sleep(500 * time.Millisecond)

	// Stop watcher
	if err := watcher.Close(); err != nil {
		t.Errorf("Failed to close watcher: %v", err)
	}
	<-watcherDone

	// Verify reload was NOT attempted because file doesn't exist
	if reloadAttempted {
		t.Fatal("Reload should not have been attempted when config file was removed without replacement")
	}

	// Verify that the original configuration is still intact
	datasources := registry.ListDatasources()
	if len(datasources) != 1 || datasources[0] != "test-timestamp" {
		t.Fatal("Original datasources should still be intact when config file is removed")
	}
}

func addDatasourceToWarehouseForTest(wh *warehouse.Warehouse, registry *core.Registry, cfg *config.Config, name string) error {
	dsType, dsConfigRaw, err := cfg.GetDatasourceConfig(name)
	if err != nil {
		return fmt.Errorf("getting config for datasource %s: %w", name, err)
	}

	if err := registry.CreateDatasource(name, dsType, nil); err != nil {
		return fmt.Errorf("creating datasource %s: %w", name, err)
	}

	datasources := registry.GetAllDatasources()
	ds, exists := datasources[name]
	if !exists {
		return fmt.Errorf("datasource %s not found after creation", name)
	}

	dsConfig, err := convertRawConfigToType(ds, dsConfigRaw)
	if err != nil {
		return fmt.Errorf("converting config for datasource %s: %w", name, err)
	}

	if err := ds.SetConfig(dsConfig); err != nil {
		return fmt.Errorf("setting config for datasource %s: %w", name, err)
	}

	interval := cfg.GetDatasourceInterval(name)
	if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
		return fmt.Errorf("adding datasource %s to warehouse: %w", name, err)
	}

	return nil
}
