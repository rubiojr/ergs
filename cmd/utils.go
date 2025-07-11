package cmd

import (
	"fmt"

	"github.com/pelletier/go-toml/v2"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

// createDatasourcesFromConfig creates and configures datasources from the config
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

// convertRawConfigToType converts raw config to the datasource's expected type
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

// initializeDatasourceStorage initializes storage for all registered datasources
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
