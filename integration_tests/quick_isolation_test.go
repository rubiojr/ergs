package integration_tests

import (
	"context"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/warehouse"

	// Import datasources to register their factories
	_ "github.com/rubiojr/ergs/pkg/datasources/codeberg"
	gasstations "github.com/rubiojr/ergs/pkg/datasources/gasstations"
	_ "github.com/rubiojr/ergs/pkg/datasources/github"
)

// TestQuickDatasourceIsolation is a fast test to verify datasource isolation
// This test is designed for CI/CD pipelines and quick validation
func TestQuickDatasourceIsolation(t *testing.T) {
	tempDir := t.TempDir()

	// Create minimal test configuration with small radius for fast execution
	testConfig := CreateTestConfigMinimal(tempDir)

	// Create registry and datasources
	registry := core.GetGlobalRegistry()
	defer registry.Close()

	// Create datasources from config
	for name, dsConfig := range testConfig.Datasources {
		// Create with nil config first
		err := registry.CreateDatasource(name, dsConfig.Type, nil)
		if err != nil {
			t.Fatalf("Failed to create datasource %s: %v", name, err)
		}

		// Set config for gas stations with proper coordinates
		if dsConfig.Type == "gasstations" {
			ds, err := registry.GetDatasource(name)
			if err != nil {
				t.Fatalf("Failed to get datasource %s: %v", name, err)
			}

			// Create proper gas station config with test coordinates
			gasConfig := &gasstations.Config{
				Latitude:  41.7664, // Default to Soria coordinates for testing
				Longitude: -2.4792,
				Radius:    2000.0, // Small radius for fast tests
			}

			// Override with specific coordinates if available
			if configMap, ok := dsConfig.Config.(map[string]interface{}); ok {
				if lat, ok := configMap["latitude"].(float64); ok {
					gasConfig.Latitude = lat
				}
				if lng, ok := configMap["longitude"].(float64); ok {
					gasConfig.Longitude = lng
				}
				if radius, ok := configMap["radius"].(float64); ok {
					gasConfig.Radius = radius
				}
			}

			err = ds.SetConfig(gasConfig)
			if err != nil {
				t.Fatalf("Failed to set config for datasource %s: %v", name, err)
			}
		}
	}

	// Create storage manager and warehouse
	storageManager := storage.NewManager(testConfig.StorageDir)
	defer storageManager.Close()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: 0, // No optimization for test
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer wh.Close()

	// Initialize datasource storage
	datasources := registry.GetAllDatasources()
	for name, ds := range datasources {
		schema := ds.Schema()
		if err := storageManager.InitializeDatasourceStorage(name, schema); err != nil {
			t.Fatalf("Failed to initialize storage for %s: %v", name, err)
		}

		// Add to warehouse
		interval := time.Hour // Use 1 hour interval for test
		if err := wh.AddDatasourceWithInterval(name, ds, interval); err != nil {
			t.Fatalf("Failed to add datasource to warehouse: %v", err)
		}

		// Register block prototype
		storageManager.RegisterBlockPrototype(name, ds.BlockPrototype())
	}

	// Fetch data from all datasources
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Minute)
	defer cancel()

	t.Log("Fetching data from datasources...")
	err := wh.FetchOnce(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}

	// Get minimal test cases
	testCases := GetMinimalTestCases()

	// Test 1: Verify basic isolation - each datasource should have its own data
	for _, tc := range testCases {
		t.Run("Isolation_"+tc.Name, func(t *testing.T) {
			// Get blocks from this specific datasource
			blocks, err := storageManager.SearchBlocks(tc.Name, "", 100)
			if err != nil {
				t.Fatalf("Failed to search blocks for %s: %v", tc.Name, err)
			}

			// Verify we have some data
			if len(blocks) == 0 {
				t.Errorf("No blocks found for datasource %s", tc.Name)
				return
			}

			// Verify all blocks have the correct source (should match datasource instance name)
			wrongSourceCount := 0
			for _, block := range blocks {
				if block.Source() != tc.Name {
					wrongSourceCount++
					if wrongSourceCount == 1 { // Only log first few errors
						t.Errorf("Block has incorrect source: expected %s, got %s", tc.Name, block.Source())
					}
				}
			}

			if wrongSourceCount > 0 {
				t.Errorf("Found %d blocks with incorrect source in datasource %s", wrongSourceCount, tc.Name)
			}

			t.Logf("Datasource %s: %d blocks verified", tc.Name, len(blocks))
		})
	}

	// Test 2: Verify search isolation
	t.Run("SearchIsolation", func(t *testing.T) {
		for _, tc := range testCases {
			// Search for location-specific term in specific datasource
			blocks, err := storageManager.SearchBlocks(tc.Name, tc.LocationKeyword, 5)
			if err != nil {
				t.Errorf("Failed to search for %s in %s: %v", tc.LocationKeyword, tc.Name, err)
				continue
			}

			// Verify all returned blocks are from the correct datasource
			for _, block := range blocks {
				if block.Source() != tc.Name {
					t.Errorf("Search returned block from wrong datasource: expected %s, got %s",
						tc.Name, block.Source())
				}
			}

			t.Logf("Search for %s in %s returned %d results", tc.LocationKeyword, tc.Name, len(blocks))
		}
	})

	// Test 3: Verify Type() vs Name() methods work correctly
	t.Run("TypeVsNameMethods", func(t *testing.T) {
		for instanceName, ds := range datasources {
			// All gas station datasources should have Type() = "gasstations"
			if ds.Type() != "gasstations" {
				t.Errorf("Expected Type() to return 'gasstations', got '%s' for %s", ds.Type(), instanceName)
			}

			// Name() should return the instance name
			if ds.Name() != instanceName {
				t.Errorf("Expected Name() to return '%s', got '%s'", instanceName, ds.Name())
			}
		}
	})

	t.Log("Quick isolation test completed successfully")
}

// TestDatasourceFactoryInstanceName tests that datasource factories receive and use instance names correctly
func TestDatasourceFactoryInstanceName(t *testing.T) {
	registry := core.GetGlobalRegistry()
	defer registry.Close()

	testCases := []struct {
		instanceName string
		expectedType string
	}{
		{"test_instance_1", "gasstations"},
		{"another_gas_instance", "gasstations"},
		{"third_instance", "gasstations"},
	}

	for _, tc := range testCases {
		t.Run(tc.instanceName, func(t *testing.T) {
			// Create datasource instance with nil config
			err := registry.CreateDatasource(tc.instanceName, tc.expectedType, nil)
			if err != nil {
				t.Fatalf("Failed to create datasource %s: %v", tc.instanceName, err)
			}

			// Set default config
			ds, err := registry.GetDatasource(tc.instanceName)
			if err != nil {
				t.Fatalf("Failed to get datasource %s: %v", tc.instanceName, err)
			}

			// Create proper gas station config with test coordinates
			gasConfig := &gasstations.Config{
				Latitude:  41.7664, // Soria coordinates for testing
				Longitude: -2.4792,
				Radius:    1000.0, // Small radius for test
			}

			err = ds.SetConfig(gasConfig)
			if err != nil {
				t.Fatalf("Failed to set config for datasource %s: %v", tc.instanceName, err)
			}

			// Verify methods return correct values
			if ds.Type() != tc.expectedType {
				t.Errorf("Expected Type() to return %s, got %s", tc.expectedType, ds.Type())
			}

			if ds.Name() != tc.instanceName {
				t.Errorf("Expected Name() to return %s, got %s", tc.instanceName, ds.Name())
			}

			t.Logf("Verified datasource %s: Type=%s, Name=%s", tc.instanceName, ds.Type(), ds.Name())
		})
	}
}
