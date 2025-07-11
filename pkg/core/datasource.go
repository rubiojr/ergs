package core

import (
	"context"
)

// Datasource represents a source of data that can fetch blocks for indexing and search.
// All datasources in Ergs must implement this interface to integrate with the system.
//
// Datasources are self-contained units that:
// - Know how to fetch data from their specific source (API, database, file, etc.)
// - Define their own data schema for storage
// - Provide block prototypes for data reconstruction
// - Manage their own configuration and lifecycle
//
// Key concepts:
// - Type vs Name: Type is the datasource category (e.g., "github"), Name is the instance (e.g., "work_github")
// - Streaming: Data is fetched and sent through channels for real-time processing
// - Self-reconstruction: Blocks can recreate themselves from stored data
// - Configuration: Datasources validate and manage their own settings
//
// Example implementation pattern:
//
//	type MyDatasource struct {
//		config       *Config
//		client       *http.Client
//		instanceName string
//	}
//
//	func (d *MyDatasource) Type() string { return "myapi" }
//	func (d *MyDatasource) Name() string { return d.instanceName }
//	// ... implement other methods
//
// Registration pattern:
//
//	func init() {
//		prototype := &MyDatasource{}
//		RegisterDatasourcePrototype("myapi", prototype)
//	}
type Datasource interface {
	// Type returns the datasource type identifier.
	// This should be a constant string that identifies the kind of datasource
	// (e.g., "github", "firefox", "gasstations").
	// Used for factory registration and configuration matching.
	Type() string

	// Name returns the unique instance name for this datasource.
	// This distinguishes between different instances of the same type
	// (e.g., "work_github" vs "personal_github" for two GitHub datasources).
	// This is what users see in search results and what ensures data isolation.
	Name() string

	// FetchBlocks retrieves data from the source and streams it as blocks.
	// This is the core method where datasources do their work.
	//
	// Implementation guidelines:
	// - Respect context cancellation for clean shutdowns
	// - Send blocks through the channel as soon as they're created
	// - Handle rate limiting and retries appropriately
	// - Log progress for user visibility
	// - Close the channel when done (handled by caller)
	//
	// Example pattern:
	//	for item := range dataSource.Items() {
	//		select {
	//		case <-ctx.Done():
	//			return ctx.Err()
	//		case blockCh <- d.convertToBlock(item):
	//		}
	//	}
	FetchBlocks(ctx context.Context, blockCh chan<- Block) error

	// Schema defines the database schema for blocks from this datasource.
	// Returns a map of field names to SQLite column types.
	//
	// Supported types: "TEXT", "INTEGER", "REAL", "BLOB"
	// These fields become searchable and are stored in the FTS table.
	//
	// Example:
	//	return map[string]any{
	//		"title":    "TEXT",
	//		"author":   "TEXT",
	//		"score":    "INTEGER",
	//		"created":  "TEXT",
	//	}
	Schema() map[string]any

	// BlockPrototype returns a prototype block for reconstruction.
	// The returned block's Factory() method will be used to recreate
	// blocks of this type when loading from the database.
	//
	// Simply return an empty instance of your block type:
	//	return &MyBlock{}
	BlockPrototype() Block

	// ConfigType returns a pointer to an empty configuration struct.
	// Used by the system to create and validate configurations.
	// Should return the same type that SetConfig() expects.
	//
	// Example:
	//	return &MyConfig{}
	ConfigType() interface{}

	// SetConfig updates the datasource configuration.
	// Called during initialization and when configuration changes.
	// Should validate the config and return an error if invalid.
	//
	// Implementation pattern:
	//	if cfg, ok := config.(*MyConfig); ok {
	//		if err := cfg.Validate(); err != nil {
	//			return err
	//		}
	//		d.config = cfg
	//		return nil
	//	}
	//	return fmt.Errorf("invalid config type")
	SetConfig(config interface{}) error

	// GetConfig returns the current configuration.
	// Used by the system to inspect or serialize current settings.
	GetConfig() interface{}

	// Close performs cleanup when the datasource is no longer needed.
	// Called during system shutdown or when removing a datasource.
	// Should close any open connections, files, or other resources.
	//
	// Example:
	//	if d.client != nil {
	//		d.client.CloseIdleConnections()
	//	}
	//	return nil
	Close() error

	// Factory creates new instances of this datasource type.
	// Called by the core system when creating datasource instances.
	// This method replaced the external factory function pattern for better encapsulation.
	//
	// Parameters:
	//   - instanceName: Unique name for this datasource instance
	//   - config: Configuration object (may be nil for defaults)
	//
	// Should validate the config and return a fully initialized datasource.
	// The datasource should be ready to use after this call.
	//
	// Example:
	//	func (d *MyDatasource) Factory(instanceName string, config interface{}) (Datasource, error) {
	//		return NewMyDatasource(instanceName, config)
	//	}
	Factory(instanceName string, config interface{}) (Datasource, error)
}

// DatasourceFactory is the legacy function type, maintained for compatibility.
// New implementations should use the Factory() method on the Datasource interface.
//
// This type is deprecated and will be removed in a future version.
// It's kept for backward compatibility during the transition period.
type DatasourceFactory func(instanceName string, config interface{}) (Datasource, error)
