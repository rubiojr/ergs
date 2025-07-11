// Package timestamp provides the simplest possible datasource implementation
// that demonstrates core Ergs datasource patterns by storing timestamp data.
//
// This datasource serves as a reference implementation for developers creating
// new datasources, showing the minimal required structure and interfaces.
//
// Features:
// - Generates blocks containing current timestamp information
// - Configurable interval (though only used during fetch operations)
// - Demonstrates proper source naming and metadata handling
// - Shows block factory pattern for database reconstruction
//
// Example configuration:
//
//	[datasources.my_timestamp]
//	type = 'timestamp'
//	interval = '5m0s'
//	[datasources.my_timestamp.config]
//	interval_seconds = 60
package timestamp

import (
	"context"
	"fmt"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// init registers this datasource with the core system.
// This is called automatically when the package is imported.
func init() {
	// Register a prototype instance for factory creation
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("timestamp", prototype)
}

// Config defines the configuration structure for the timestamp datasource.
// This demonstrates the minimal configuration pattern - just one optional field.
type Config struct {
	// IntervalSeconds is currently not used during operation but shows
	// how to include configurable parameters. In a real datasource,
	// this might control polling frequency or batch sizes.
	IntervalSeconds int `toml:"interval_seconds"`
}

// Validate ensures the configuration is valid and sets defaults.
// This pattern should be used by all datasources to provide sensible defaults.
func (c *Config) Validate() error {
	if c.IntervalSeconds <= 0 {
		c.IntervalSeconds = 60 // Default to 60 seconds
	}
	return nil
}

// Datasource implements the core.Datasource interface for timestamp data.
// This shows the minimal structure needed for any datasource.
type Datasource struct {
	config       *Config // Configuration specific to this datasource
	instanceName string  // The unique name for this datasource instance
}

// NewDatasource creates a new timestamp datasource instance.
// This constructor pattern is standard across all Ergs datasources.
//
// Parameters:
//   - instanceName: The unique identifier for this datasource instance
//   - config: Configuration object (can be nil for defaults)
//
// Returns the configured datasource or an error if configuration is invalid.
func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var timestampConfig *Config

	// Handle nil config by providing sensible defaults
	if config == nil {
		timestampConfig = &Config{
			IntervalSeconds: 60,
		}
	} else {
		// Type assertion to ensure we have the correct config type
		var ok bool
		timestampConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for timestamp datasource")
		}
	}

	return &Datasource{
		config:       timestampConfig,
		instanceName: instanceName,
	}, nil
}

// Type returns the datasource type identifier.
// This should match the type used in configuration files and factory registration.
func (d *Datasource) Type() string {
	return "timestamp"
}

// Name returns the instance name for this datasource.
// This distinguishes between different instances of the same datasource type.
// IMPORTANT: After recent fixes, this returns the instance name, not the type.
func (d *Datasource) Name() string {
	return d.instanceName
}

// Schema defines the database schema for blocks created by this datasource.
// Each field will become a column in the FTS (Full-Text Search) table.
// Use appropriate SQLite types: TEXT, INTEGER, REAL, BLOB.
func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"timestamp": "TEXT",    // ISO 8601 formatted timestamp
		"unix":      "INTEGER", // Unix timestamp for easy sorting/filtering
	}
}

// BlockPrototype returns a prototype block used for reconstruction from database data.
// This is essential for loading saved blocks back into memory.
func (d *Datasource) BlockPrototype() core.Block {
	return &TimestampBlock{}
}

// ConfigType returns a pointer to an empty config struct.
// This is used by the configuration system to create and validate configs.
func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

// SetConfig updates the datasource configuration.
// This includes validation to ensure the config is valid.
func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		d.config = cfg
		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for timestamp datasource")
}

// GetConfig returns the current configuration.
// Used by the system to inspect or serialize current settings.
func (d *Datasource) GetConfig() interface{} {
	return d.config
}

// FetchBlocks is the core method that retrieves data and converts it to blocks.
// This method is called by the Ergs system during fetch operations.
//
// For the timestamp datasource, we simply create a single block with the current time.
// Real datasources would typically:
// 1. Make API calls or read from databases/files
// 2. Process the data into multiple blocks
// 3. Handle pagination and rate limiting
// 4. Send blocks through the channel as they're created
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - blockCh: Channel to send created blocks through
//
// The method should respect context cancellation and close cleanly.
func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	now := time.Now()

	// Create a single block with current timestamp
	// CRITICAL: Use instance name as source, not datasource type
	block := NewTimestampBlockWithSource(
		now,
		d.instanceName, // This ensures proper data isolation between instances
	)

	// Always check for cancellation before blocking operations
	select {
	case <-ctx.Done():
		return ctx.Err()
	case blockCh <- block:
		return nil
	}
}

// Close performs cleanup when the datasource is no longer needed.
// For simple datasources like this one, no cleanup is required.
// Real datasources might close database connections, HTTP clients, etc.
func (d *Datasource) Close() error {
	return nil
}

// Factory creates a new instance of this datasource.
// This method is part of the core.Datasource interface and is called
// by the core system when creating datasource instances.
func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
