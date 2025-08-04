package integration_tests

import (
	"testing"

	"github.com/rubiojr/ergs/pkg/core"
	_ "github.com/rubiojr/ergs/pkg/datasources/testrand"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
)

func TestDatasourceRegistration(t *testing.T) {
	// Get the global registry (datasources should be registered via init() functions)
	registry := core.GetGlobalRegistry()

	// Check that expected datasources are registered
	expectedDatasources := []string{"testrand", "timestamp"}

	for _, dsName := range expectedDatasources {
		t.Run("datasource_"+dsName, func(t *testing.T) {
			// Create a datasource with empty config first
			err := registry.CreateDatasource("test-"+dsName, dsName, nil)
			if err != nil {
				t.Errorf("Failed to create datasource %s: %v", dsName, err)
				return
			}

			// Get the datasource
			datasources := registry.GetAllDatasources()
			ds, exists := datasources["test-"+dsName]
			if !exists {
				t.Errorf("Datasource %s was not found after creation", dsName)
				return
			}

			// Verify the datasource has the expected interface methods
			expectedInstanceName := "test-" + dsName
			if ds.Name() != expectedInstanceName {
				t.Errorf("Expected datasource name '%s', got '%s'", expectedInstanceName, ds.Name())
			}

			// Verify the datasource type is correct
			if ds.Type() != dsName {
				t.Errorf("Expected datasource type '%s', got '%s'", dsName, ds.Type())
			}

			if ds.Schema() == nil {
				t.Errorf("Datasource %s should have a schema", dsName)
			}

			if ds.BlockPrototype() == nil {
				t.Errorf("Datasource %s should have a block prototype", dsName)
			}

			if ds.ConfigType() == nil {
				t.Errorf("Datasource %s should have a config type", dsName)
			}

			// Test that we can set config using the proper config type
			configType := ds.ConfigType()
			err = ds.SetConfig(configType)
			if err != nil {
				t.Errorf("Failed to set config for datasource %s: %v", dsName, err)
			}

			// Test that we can get config
			if ds.GetConfig() == nil {
				t.Errorf("Datasource %s should return config after setting it", dsName)
			}
		})
	}
}

func TestRegistryIsolation(t *testing.T) {
	// Get two registry instances
	registry1 := core.GetGlobalRegistry()
	registry2 := core.GetGlobalRegistry()

	// Create a datasource in one registry
	err := registry1.CreateDatasource("test-isolation", "timestamp", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource in registry1: %v", err)
	}

	// Verify it doesn't affect the other registry (they should be independent instances)
	datasources2 := registry2.GetAllDatasources()
	if _, exists := datasources2["test-isolation"]; exists {
		t.Error("Datasource should not exist in registry2 - registries should be independent")
	}
}

func TestDatasourceFactoriesAvailable(t *testing.T) {
	registry := core.GetGlobalRegistry()

	// Verify that both testrand and timestamp factories are available
	expectedFactories := map[string]bool{
		"testrand":  false,
		"timestamp": false,
	}

	// Try to create each datasource type
	for dsType := range expectedFactories {
		err := registry.CreateDatasource("test-"+dsType, dsType, nil)
		if err == nil {
			expectedFactories[dsType] = true
		}
	}

	// Check that all expected factories were found
	for dsType, found := range expectedFactories {
		if !found {
			t.Errorf("Datasource factory '%s' was not registered", dsType)
		}
	}
}
