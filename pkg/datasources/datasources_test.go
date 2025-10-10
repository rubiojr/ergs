package datasources

import (
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	_ "github.com/rubiojr/ergs/pkg/datasources/chromium"
	_ "github.com/rubiojr/ergs/pkg/datasources/codeberg"
	_ "github.com/rubiojr/ergs/pkg/datasources/datadis"
	_ "github.com/rubiojr/ergs/pkg/datasources/firefox"
	_ "github.com/rubiojr/ergs/pkg/datasources/gasstations"
	_ "github.com/rubiojr/ergs/pkg/datasources/github"
	_ "github.com/rubiojr/ergs/pkg/datasources/hackernews"
	_ "github.com/rubiojr/ergs/pkg/datasources/homeassistant"
	_ "github.com/rubiojr/ergs/pkg/datasources/importer"
	_ "github.com/rubiojr/ergs/pkg/datasources/rss"
	_ "github.com/rubiojr/ergs/pkg/datasources/rtve"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
	_ "github.com/rubiojr/ergs/pkg/datasources/zedthreads"
)

// TestAllDatasourcesHandleNilConfig ensures all datasources can be created with nil config
// This is critical because the registry creates datasources with nil config during initialization
func TestAllDatasourcesHandleNilConfig(t *testing.T) {
	registry := core.GetGlobalRegistry()

	// Dynamically discover all registered datasource types
	datasourceTypes := registry.ListPrototypeTypes()

	if len(datasourceTypes) == 0 {
		t.Fatal("No datasources registered - check imports")
	}

	for _, dsType := range datasourceTypes {
		t.Run(dsType, func(t *testing.T) {
			// This is what the registry does during initialization
			err := registry.CreateDatasource("test-instance", dsType, nil)
			if err != nil {
				t.Errorf("Datasource %s failed to create with nil config: %v", dsType, err)
				return
			}

			ds, err := registry.GetDatasource("test-instance")
			if err != nil {
				t.Errorf("Failed to get datasource after creation: %v", err)
				return
			}

			if ds == nil {
				t.Errorf("Datasource %s returned nil with no error", dsType)
				return
			}

			// Verify basic interface methods work
			if ds.Name() != "test-instance" {
				t.Errorf("Datasource %s Name() returned %q, expected %q", dsType, ds.Name(), "test-instance")
			}

			if ds.Type() != dsType {
				t.Errorf("Datasource %s Type() returned %q, expected %q", dsType, ds.Type(), dsType)
			}

			// Schema should be available (except for importer which returns nil)
			schema := ds.Schema()
			if dsType != "importer" && schema == nil {
				t.Errorf("Datasource %s Schema() returned nil", dsType)
			}

			// BlockPrototype should always be available
			prototype := ds.BlockPrototype()
			if prototype == nil {
				t.Errorf("Datasource %s BlockPrototype() returned nil", dsType)
			}

			// ConfigType should always be available
			configType := ds.ConfigType()
			if configType == nil {
				t.Errorf("Datasource %s ConfigType() returned nil", dsType)
			}

			// GetConfig should return something (even if empty config)
			config := ds.GetConfig()
			if config == nil {
				t.Errorf("Datasource %s GetConfig() returned nil", dsType)
			}

			// Close should not error
			if err := ds.Close(); err != nil {
				t.Errorf("Datasource %s Close() returned error: %v", dsType, err)
			}

			// Clean up
			_ = registry.RemoveDatasource("test-instance")
		})
	}
}

// TestAllDatasourcesHaveValidSchema ensures all datasources define proper schemas
func TestAllDatasourcesHaveValidSchema(t *testing.T) {
	registry := core.GetGlobalRegistry()

	// Dynamically discover all registered datasource types
	datasourceTypes := registry.ListPrototypeTypes()

	for _, dsType := range datasourceTypes {
		t.Run(dsType, func(t *testing.T) {
			err := registry.CreateDatasource("test-instance", dsType, nil)
			if err != nil {
				t.Fatalf("Failed to create datasource: %v", err)
			}
			defer func() {
				_ = registry.RemoveDatasource("test-instance")
			}()

			ds, err := registry.GetDatasource("test-instance")
			if err != nil {
				t.Fatalf("Failed to get datasource: %v", err)
			}

			schema := ds.Schema()

			// Importer is a special case - it has no schema because it routes to other datasources
			if dsType == "importer" {
				if schema != nil {
					t.Errorf("Datasource importer should return nil schema but got %v", schema)
				}
				return
			}

			if len(schema) == 0 {
				t.Errorf("Datasource %s has empty schema - should define at least one field", dsType)
			}

			// Verify schema values are valid SQL types
			validTypes := map[string]bool{
				"TEXT":    true,
				"INTEGER": true,
				"REAL":    true,
				"BOOLEAN": true,
				"BLOB":    true,
			}

			for field, sqlType := range schema {
				sqlTypeStr, ok := sqlType.(string)
				if !ok {
					t.Errorf("Datasource %s schema field %q has non-string type: %T", dsType, field, sqlType)
					continue
				}

				if !validTypes[sqlTypeStr] {
					t.Errorf("Datasource %s schema field %q has invalid SQL type %q", dsType, field, sqlTypeStr)
				}
			}
		})
	}
}

// TestAllDatasourcesHaveValidBlockPrototype ensures block prototypes implement required methods
func TestAllDatasourcesHaveValidBlockPrototype(t *testing.T) {
	registry := core.GetGlobalRegistry()

	// Dynamically discover all registered datasource types
	datasourceTypes := registry.ListPrototypeTypes()

	for _, dsType := range datasourceTypes {
		t.Run(dsType, func(t *testing.T) {
			err := registry.CreateDatasource("test-instance", dsType, nil)
			if err != nil {
				t.Fatalf("Failed to create datasource: %v", err)
			}
			defer func() {
				_ = registry.RemoveDatasource("test-instance")
			}()

			ds, err := registry.GetDatasource("test-instance")
			if err != nil {
				t.Fatalf("Failed to get datasource: %v", err)
			}

			prototype := ds.BlockPrototype()
			if prototype == nil {
				t.Fatal("BlockPrototype returned nil")
			}

			// Verify prototype has a Factory method that works
			// Factory should accept a GenericBlock
			genericBlock := core.NewGenericBlock(
				"test-id",
				"test text",
				"test-source",
				dsType,
				time.Now(),
				map[string]interface{}{},
			)

			// This should not panic
			reconstructed := prototype.Factory(genericBlock, "test-source")
			if reconstructed == nil {
				t.Error("Factory() returned nil block")
			}

			// Verify basic interface methods work on reconstructed block
			if reconstructed.Type() != dsType {
				t.Errorf("Reconstructed block Type() = %q, want %q", reconstructed.Type(), dsType)
			}

			if reconstructed.Source() != "test-source" {
				t.Errorf("Reconstructed block Source() = %q, want %q", reconstructed.Source(), "test-source")
			}

			// Verify Summary doesn't panic
			summary := reconstructed.Summary()
			if summary == "" {
				t.Errorf("Reconstructed block Summary() returned empty string")
			}
		})
	}
}

// TestAllDatasourcesHandleSetConfigProperly ensures SetConfig handles nil/invalid configs
func TestAllDatasourcesHandleSetConfigProperly(t *testing.T) {
	registry := core.GetGlobalRegistry()

	// Dynamically discover all registered datasource types
	datasourceTypes := registry.ListPrototypeTypes()

	for _, dsType := range datasourceTypes {
		t.Run(dsType+"/InvalidConfig", func(t *testing.T) {
			err := registry.CreateDatasource("test-instance", dsType, nil)
			if err != nil {
				t.Fatalf("Failed to create datasource: %v", err)
			}
			defer func() {
				_ = registry.RemoveDatasource("test-instance")
			}()

			ds, err := registry.GetDatasource("test-instance")
			if err != nil {
				t.Fatalf("Failed to get datasource: %v", err)
			}

			// SetConfig with wrong type should return error
			err = ds.SetConfig("invalid config type")
			if err == nil {
				t.Error("SetConfig should return error for invalid config type")
			}
		})

		t.Run(dsType+"/ValidConfigType", func(t *testing.T) {
			err := registry.CreateDatasource("test-instance", dsType, nil)
			if err != nil {
				t.Fatalf("Failed to create datasource: %v", err)
			}
			defer func() {
				_ = registry.RemoveDatasource("test-instance")
			}()

			ds, err := registry.GetDatasource("test-instance")
			if err != nil {
				t.Fatalf("Failed to get datasource: %v", err)
			}

			// Get a valid config instance
			configType := ds.ConfigType()

			// SetConfig with proper type instance should not panic
			// (might error due to validation, but shouldn't panic)
			err = ds.SetConfig(configType)
			// We don't require it to succeed with empty config, but it shouldn't panic
			_ = err
		})
	}
}

// TestAllDatasourcesRegistered ensures we have at least the core datasources registered
func TestAllDatasourcesRegistered(t *testing.T) {
	registry := core.GetGlobalRegistry()

	// Dynamically discover all registered datasource types
	datasourceTypes := registry.ListPrototypeTypes()

	if len(datasourceTypes) == 0 {
		t.Fatal("No datasources registered - check imports")
	}

	// Test that we have at least some expected core datasources
	// This ensures the imports are working
	expectedCoreTypes := map[string]bool{
		"github":   true,
		"firefox":  true,
		"chromium": true,
		"datadis":  true,
	}

	registeredTypes := make(map[string]bool)
	for _, dsType := range datasourceTypes {
		registeredTypes[dsType] = true
	}

	for expectedType := range expectedCoreTypes {
		if !registeredTypes[expectedType] {
			t.Errorf("Expected core datasource %q is not registered", expectedType)
		}
	}

	t.Logf("Found %d registered datasource types: %v", len(datasourceTypes), datasourceTypes)
}
