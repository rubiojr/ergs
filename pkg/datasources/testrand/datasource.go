// Package testrand provides a random data generator datasource for testing.
//
// This datasource generates random blocks with configurable properties,
// making it ideal for testing datasource isolation, search functionality,
// and other core features without requiring network access.
//
// Features:
// - Generates random text blocks with configurable count
// - No network dependencies - perfect for unit tests
// - Configurable prefixes for easy identification in tests
// - Demonstrates proper source naming and metadata handling
//
// Example configuration:
//
//	[datasources.test_random_1]
//	type = 'testrand'
//	[datasources.test_random_1.config]
//	count = 10
//	prefix = "TEST1"
package testrand

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// init registers this datasource with the core system.
func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("testrand", prototype)
}

// Config defines the configuration structure for the random test datasource.
type Config struct {
	// Count specifies how many random blocks to generate
	Count int `toml:"count"`
	// Prefix is added to each generated block for easy identification
	Prefix string `toml:"prefix"`
	// Seed for reproducible randomness (optional)
	Seed int64 `toml:"seed"`
}

// Validate ensures the configuration is valid and sets defaults.
func (c *Config) Validate() error {
	if c.Count <= 0 {
		c.Count = 5 // Default to 5 blocks
	}
	if c.Prefix == "" {
		c.Prefix = "RAND"
	}
	return nil
}

// Datasource implements the core.Datasource interface for random test data.
type Datasource struct {
	config       *Config
	instanceName string
	rng          *rand.Rand
}

// NewDatasource creates a new random test datasource instance.
func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var randConfig *Config

	if config == nil {
		randConfig = &Config{
			Count:  5,
			Prefix: "RAND",
			Seed:   time.Now().UnixNano(),
		}
	} else {
		var ok bool
		randConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for testrand datasource")
		}
	}

	// Validate the config
	if err := randConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid config: %w", err)
	}

	// Create seeded random number generator for reproducible tests
	seed := randConfig.Seed
	if seed == 0 {
		seed = time.Now().UnixNano()
	}

	return &Datasource{
		config:       randConfig,
		instanceName: instanceName,
		rng:          rand.New(rand.NewSource(seed)),
	}, nil
}

// Type returns the datasource type identifier.
func (d *Datasource) Type() string {
	return "testrand"
}

// Name returns the instance name for this datasource.
func (d *Datasource) Name() string {
	return d.instanceName
}

// Schema defines the database schema for blocks created by this datasource.
func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"block_id":    "TEXT",    // Unique identifier for the block
		"random_data": "TEXT",    // Random text content
		"prefix":      "TEXT",    // The configured prefix
		"sequence":    "INTEGER", // Sequence number within the batch
	}
}

// BlockPrototype returns a prototype block used for reconstruction.
func (d *Datasource) BlockPrototype() core.Block {
	return &RandomBlock{}
}

// ConfigType returns a pointer to an empty config struct.
func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

// SetConfig updates the datasource configuration.
func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		if err := cfg.Validate(); err != nil {
			return err
		}
		d.config = cfg
		return nil
	}
	return fmt.Errorf("invalid config type for testrand datasource")
}

// GetConfig returns the current configuration.
func (d *Datasource) GetConfig() interface{} {
	return d.config
}

// FetchBlocks generates random blocks according to configuration.
func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	for i := 0; i < d.config.Count; i++ {
		// Check for cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Generate random content
		randomWords := d.generateRandomText()

		block := NewRandomBlockWithSource(
			fmt.Sprintf("%s_%s_%d", d.config.Prefix, d.instanceName, i),
			randomWords,
			d.config.Prefix,
			i,
			time.Now(),
			d.instanceName, // Use instance name as source
		)

		// Send the block
		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
		}
	}

	return nil
}

// generateRandomText creates random text content for testing.
func (d *Datasource) generateRandomText() string {
	words := []string{
		"lorem", "ipsum", "dolor", "sit", "amet", "consectetur", "adipiscing",
		"elit", "sed", "do", "eiusmod", "tempor", "incididunt", "ut", "labore",
		"et", "dolore", "magna", "aliqua", "enim", "ad", "minim", "veniam",
		"quis", "nostrud", "exercitation", "ullamco", "laboris", "nisi", "ut",
		"aliquip", "ex", "ea", "commodo", "consequat", "duis", "aute", "irure",
		"test", "data", "random", "generated", "block", "content", "sample",
	}

	// Generate 3-8 random words
	wordCount := 3 + d.rng.Intn(6)
	var result []string

	for i := 0; i < wordCount; i++ {
		word := words[d.rng.Intn(len(words))]
		result = append(result, word)
	}

	return fmt.Sprintf("%s %s", d.config.Prefix, joinWords(result))
}

// joinWords joins a slice of words with spaces.
func joinWords(words []string) string {
	if len(words) == 0 {
		return ""
	}

	result := words[0]
	for i := 1; i < len(words); i++ {
		result += " " + words[i]
	}

	return result
}

// Close performs cleanup when the datasource is no longer needed.
func (d *Datasource) Close() error {
	return nil
}

// Factory creates a new instance of this datasource.
func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
