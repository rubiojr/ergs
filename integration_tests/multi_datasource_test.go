package integration_tests

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
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

// These integration tests prevent a critical cross-contamination bug that was discovered
// where multiple instances of the same datasource type (e.g., soria_gas, madrid_gas,
// zaragoza_gas) would store data in the wrong databases during concurrent fetching.
//
// The bug occurred because:
// 1. All gas station blocks had source = "gasstations" (datasource type)
// 2. Warehouse storeBlock() used ds.Name() == block.Source() lookup
// 3. All gas station datasources had ds.Name() = "gasstations"
// 4. First matching datasource received ALL gas station blocks
//
// The fix involved:
// 1. Adding Type() method for datasource type ("gasstations")
// 2. Using Name() method for instance name ("soria_gas")
// 3. Setting block source to instance name instead of type name
// 4. Simplifying warehouse storage logic to use block.Source() directly
//
// These tests ensure this architectural issue cannot regress.

// TestMultipleDatasourceIsolation tests that multiple instances of the same datasource type
// don't cross-contaminate each other's data
func TestMultipleDatasourceIsolation(t *testing.T) {
	// Create temporary directory for test
	tempDir := t.TempDir()

	// Create test configuration with multiple gas station datasources
	testConfig := CreateTestConfig(tempDir)

	// Create registry and datasources
	registry := core.GetGlobalRegistry()
	defer func() {
		if err := registry.Close(); err != nil {
			t.Logf("Warning: failed to close registry: %v", err)
		}
	}()

	// Create datasources from config
	for name, dsConfig := range testConfig.Datasources {
		// Create with nil config first
		err := registry.CreateDatasource(name, dsConfig.Type, nil)
		if err != nil {
			t.Fatalf("Failed to create datasource %s: %v", name, err)
		}

		// For gas stations, create proper config with coordinates
		if dsConfig.Type == "gasstations" {
			ds, err := registry.GetDatasource(name)
			if err != nil {
				t.Fatalf("Failed to get datasource %s: %v", name, err)
			}

			// Create proper gas station config with test coordinates
			gasConfig := &gasstations.Config{
				Latitude:  41.7664, // Default to Soria coordinates for testing
				Longitude: -2.4792,
				Radius:    5000.0,
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
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	warehouseConfig := warehouse.Config{
		OptimizeInterval: 0, // No optimization for test
	}
	wh := warehouse.NewWarehouse(warehouseConfig, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Logf("Warning: failed to close warehouse: %v", err)
		}
	}()

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
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()

	t.Log("Fetching data from all gas station datasources...")
	err := wh.FetchOnce(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch data: %v", err)
	}

	// Test 1: Verify database files exist for each datasource
	expectedDatabases := []string{"test_soria_gas.db", "test_madrid_gas.db", "test_zaragoza_gas.db"}
	for _, dbName := range expectedDatabases {
		dbPath := filepath.Join(tempDir, dbName)
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Errorf("Database file %s does not exist", dbName)
		}
	}

	// Test 2: Verify each database has data and check data isolation
	datasourceTests := GetStandardTestCases()

	totalExpectedStations := 0
	for _, test := range datasourceTests {
		t.Run(fmt.Sprintf("DataIsolation_%s", test.Name), func(t *testing.T) {
			// Get blocks from this specific datasource
			blocks, err := storageManager.SearchBlocks(test.Name, "", 1000)
			if err != nil {
				t.Fatalf("Failed to search blocks for %s: %v", test.Name, err)
			}

			// Verify we have some data
			if len(blocks) < test.ExpectedMinCount {
				t.Errorf("Expected at least %d blocks for %s, got %d",
					test.ExpectedMinCount, test.Name, len(blocks))
			}

			totalExpectedStations += len(blocks)

			// Verify all blocks have the correct source (should match datasource instance name)
			for _, block := range blocks {
				if block.Source() != test.Name {
					t.Errorf("Block has incorrect source: expected %s, got %s", test.Name, block.Source())
				}
			}

			// Verify location-specific data isolation
			locationMatches := 0
			wrongLocationBlocks := []string{}

			for _, block := range blocks {
				text := block.Text()

				// Count blocks that contain the expected location
				if ContainsLocation(text, test.LocationKeyword) {
					locationMatches++
				} else {
					// Check for contamination from other test locations
					for _, otherTest := range datasourceTests {
						if otherTest.Name != test.Name && ContainsLocation(text, otherTest.LocationKeyword) {
							wrongLocationBlocks = append(wrongLocationBlocks,
								fmt.Sprintf("Block in %s contains %s: %s",
									test.Name, otherTest.LocationKeyword, text[:100]))
						}
					}
				}
			}

			// We should have some stations in the expected location
			if locationMatches == 0 {
				t.Errorf("No blocks found containing expected location %s in datasource %s",
					test.LocationKeyword, test.Name)
			}

			// We should not have contamination from other locations
			if len(wrongLocationBlocks) > 0 {
				t.Errorf("Cross-contamination detected in %s:", test.Name)
				for _, contamination := range wrongLocationBlocks {
					t.Errorf("  %s", contamination)
				}
			}

			t.Logf("Datasource %s: %d total blocks, %d with expected location %s",
				test.Name, len(blocks), locationMatches, test.LocationKeyword)
		})
	}

	// Test 3: Verify search functionality works correctly for each datasource
	t.Run("SearchFunctionality", func(t *testing.T) {
		for _, test := range datasourceTests {
			// Search for location-specific term in specific datasource
			blocks, err := storageManager.SearchBlocks(test.Name, test.LocationKeyword, 10)
			if err != nil {
				t.Errorf("Failed to search for %s in %s: %v", test.LocationKeyword, test.Name, err)
				continue
			}

			// Verify all returned blocks are from the correct datasource
			for _, block := range blocks {
				if block.Source() != test.Name {
					t.Errorf("Search returned block from wrong datasource: expected %s, got %s",
						test.Name, block.Source())
				}

				// Verify the search term is actually in the results
				if !ContainsLocation(block.Text(), test.LocationKeyword) {
					t.Errorf("Search result doesn't contain search term %s: %s",
						test.LocationKeyword, block.Text())
				}
			}

			t.Logf("Search for %s in %s returned %d results", test.LocationKeyword, test.Name, len(blocks))
		}
	})

	// Test 4: Verify cross-datasource search doesn't mix results incorrectly
	t.Run("CrossDatasourceSearch", func(t *testing.T) {
		// Search across all datasources for a term that should only appear in one
		results, err := storageManager.SearchAllDatasources("SORIA", 50)
		if err != nil {
			t.Fatalf("Failed to perform cross-datasource search: %v", err)
		}

		// Should only find results in test_soria_gas
		if len(results) != 1 {
			var keys []string
			for k := range results {
				keys = append(keys, k)
			}
			t.Errorf("Expected results from 1 datasource, got %d: %v", len(results), keys)
		}

		if soriaResults, exists := results["test_soria_gas"]; !exists {
			t.Error("Expected results from test_soria_gas datasource")
		} else if len(soriaResults) == 0 {
			t.Error("Expected some results from test_soria_gas datasource")
		} else {
			// Verify all results are actually from Soria datasource
			for _, block := range soriaResults {
				if block.Source() != "test_soria_gas" {
					t.Errorf("Cross-datasource search returned block from wrong source: %s", block.Source())
				}
			}
			t.Logf("Cross-datasource search for SORIA found %d results in test_soria_gas", len(soriaResults))
		}
	})

	// Test 5: Verify database schema and metadata
	t.Run("DatabaseSchema", func(t *testing.T) {
		for _, test := range datasourceTests {
			storage, err := storageManager.GetStorage(test.Name)
			if err != nil {
				t.Errorf("Failed to get storage for %s: %v", test.Name, err)
				continue
			}

			// Verify database has expected tables
			rows, err := storage.ExecuteQuery("SELECT name FROM sqlite_master WHERE type='table';")
			if err != nil {
				t.Errorf("Failed to query tables for %s: %v", test.Name, err)
				continue
			}
			defer func() {
				if err := rows.Close(); err != nil {
					t.Logf("Warning: failed to close rows: %v", err)
				}
			}()

			tables := []string{}
			for rows.Next() {
				var tableName string
				if err := rows.Scan(&tableName); err != nil {
					t.Errorf("Failed to scan table name: %v", err)
					continue
				}
				tables = append(tables, tableName)
			}

			// Verify required tables exist
			requiredTables := []string{"blocks", "blocks_fts", "fetch_metadata"}
			for _, required := range requiredTables {
				found := false
				for _, table := range tables {
					if table == required {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("Required table %s not found in %s database", required, test.Name)
				}
			}

			t.Logf("Database %s has tables: %v", test.Name, tables)
		}
	})

	t.Logf("Integration test completed successfully. Total stations processed: %d", totalExpectedStations)
}

// TestDatasourceTypeVsInstanceName tests that the Type() and Name() methods
// return the correct values for datasource instances
func TestDatasourceTypeVsInstanceName(t *testing.T) {
	registry := core.GetGlobalRegistry()
	defer func() {
		if err := registry.Close(); err != nil {
			t.Logf("Warning: failed to close registry: %v", err)
		}
	}()

	// Test case: multiple gas station instances
	testCases := []struct {
		instanceName string
		expectedType string
		config       map[string]interface{}
	}{
		{
			instanceName: "my_soria_gas",
			expectedType: "gasstations",
			config: map[string]interface{}{
				"latitude":  41.7664,
				"longitude": -2.4792,
				"radius":    5000.0,
			},
		},
		{
			instanceName: "my_madrid_gas",
			expectedType: "gasstations",
			config: map[string]interface{}{
				"latitude":  40.4168,
				"longitude": -3.7038,
				"radius":    5000.0,
			},
		},
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
				Radius:    5000.0,
			}

			err = ds.SetConfig(gasConfig)
			if err != nil {
				t.Fatalf("Failed to set config for datasource %s: %v", tc.instanceName, err)
			}

			// Verify methods return correct values

			// Verify Type() returns the datasource type
			if ds.Type() != tc.expectedType {
				t.Errorf("Expected Type() to return %s, got %s", tc.expectedType, ds.Type())
			}

			// Verify Name() returns the instance name
			if ds.Name() != tc.instanceName {
				t.Errorf("Expected Name() to return %s, got %s", tc.instanceName, ds.Name())
			}

			t.Logf("Datasource %s: Type=%s, Name=%s", tc.instanceName, ds.Type(), ds.Name())
		})
	}
}

// TestBlockSourceMatching tests that blocks created by datasources
// have the correct source field matching the instance name
func TestBlockSourceMatching(t *testing.T) {
	// Skip this test in short mode as it requires network access
	if testing.Short() {
		t.Skip("Skipping integration test in short mode")
	}

	_ = t.TempDir()

	registry := core.GetGlobalRegistry()
	defer func() {
		if err := registry.Close(); err != nil {
			t.Logf("Warning: failed to close registry: %v", err)
		}
	}()

	instanceName := "test_block_source_gas"
	dsType := "gasstations"

	// Create a gas station datasource
	err := registry.CreateDatasource(instanceName, dsType, nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	ds, err := registry.GetDatasource(instanceName)
	if err != nil {
		t.Fatalf("Failed to get datasource: %v", err)
	}

	// Set test config with proper coordinates
	gasConfig := &gasstations.Config{
		Latitude:  41.7664, // Soria coordinates for testing
		Longitude: -2.4792,
		Radius:    2000.0, // Small radius for fast test
	}

	err = ds.SetConfig(gasConfig)
	if err != nil {
		t.Fatalf("Failed to set config: %v", err)
	}

	// Create a channel to collect blocks
	blockCh := make(chan core.Block, 100)

	// Fetch blocks in a goroutine
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	go func() {
		defer close(blockCh)
		err := ds.FetchBlocks(ctx, blockCh)
		if err != nil && err != context.Canceled {
			t.Errorf("Error fetching blocks: %v", err)
		}
	}()

	// Collect and verify blocks
	blocks := []core.Block{}
	for block := range blockCh {
		blocks = append(blocks, block)

		// Verify the block source matches the instance name
		if block.Source() != instanceName {
			t.Errorf("Block has incorrect source: expected %s, got %s", instanceName, block.Source())
		}
	}

	if len(blocks) == 0 {
		t.Error("No blocks were fetched")
	}

	t.Logf("Successfully verified %d blocks with correct source field: %s", len(blocks), instanceName)
}

// Helper functions - most moved to test_helpers.go
