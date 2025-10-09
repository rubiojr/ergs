// Package timestamp provides block implementation for timestamp datasource.
// This file demonstrates the core patterns for implementing blocks in Ergs:
// - Block struct with required interface methods
// - Constructor functions with proper metadata handling
// - BlockFactory for database reconstruction
// - Helper functions for safe type conversion
package timestamp

import (
	"fmt"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// TimestampBlock represents a single timestamp entry in the system.
// This demonstrates the standard block structure that all Ergs blocks should follow.
//
// Key patterns shown:
// - Required fields for core.Block interface (id, text, createdAt, source, metadata)
// - Domain-specific fields (timestamp, unix)
// - Proper metadata handling for database persistence
type TimestampBlock struct {
	// Core Block interface fields - these are required for all blocks
	id        string                 // Unique identifier for this block
	text      string                 // Searchable text content
	createdAt time.Time              // When this block was created
	source    string                 // Source datasource instance name
	metadata  map[string]interface{} // Structured data for database storage

	// Domain-specific fields - add whatever makes sense for your datasource
	timestamp time.Time // The actual timestamp this block represents
	unix      int64     // Unix timestamp for easy comparison/sorting
}

// NewTimestampBlock creates a new timestamp block with default source.
// This is a convenience constructor that uses the datasource type as source.
// Note: This is primarily for backward compatibility - prefer NewTimestampBlockWithSource.
func NewTimestampBlock(timestamp time.Time) *TimestampBlock {
	return NewTimestampBlockWithSource(timestamp, "timestamp")
}

// NewTimestampBlockWithSource creates a new timestamp block with explicit source.
// This is the preferred constructor that properly handles data isolation.
//
// Parameters:
//   - timestamp: The time value this block represents
//   - source: The datasource instance name (for proper data isolation)
//
// Key patterns demonstrated:
// - Generate searchable text from domain data
// - Store structured data in metadata for database persistence
// - Create unique but deterministic block IDs
// - Store source in metadata for proper reconstruction
func NewTimestampBlockWithSource(timestamp time.Time, source string) *TimestampBlock {
	// Generate searchable text - this is what users will find when searching
	// Include relevant terms that users might search for
	text := fmt.Sprintf("timestamp %s", timestamp.Format("2006-01-02 15:04:05"))

	// Metadata contains all structured data needed for database storage
	// This must include everything needed to reconstruct the block
	metadata := map[string]interface{}{
		"timestamp": timestamp.Format(time.RFC3339), // ISO 8601 format for parsing
		"unix":      timestamp.Unix(),               // Integer for easy comparison
		"source":    source,                         // CRITICAL: Store source for reconstruction
	}

	// Block ID should be unique but deterministic
	// Using timestamp ensures uniqueness while allowing deduplication
	blockID := fmt.Sprintf("timestamp-%d", timestamp.Unix())

	return &TimestampBlock{
		id:        blockID,
		text:      text,
		createdAt: timestamp,
		source:    source,   // Store source directly for interface compliance
		metadata:  metadata, // Store all data for database persistence
		timestamp: timestamp,
		unix:      timestamp.Unix(),
	}
}

// Core Block interface implementation - these methods are required for all blocks

// ID returns the unique identifier for this block
func (b *TimestampBlock) ID() string { return b.id }

// Text returns the searchable text content
func (b *TimestampBlock) Text() string { return b.text }

// CreatedAt returns when this block was created
func (b *TimestampBlock) CreatedAt() time.Time { return b.createdAt }

// Source returns the datasource instance name that created this block
func (b *TimestampBlock) Source() string { return b.source }

// Metadata returns structured data for database storage and reconstruction
func (b *TimestampBlock) Metadata() map[string]interface{} { return b.metadata }

func (b *TimestampBlock) Type() string {
	return "timestamp"
}

// PrettyText returns a human-readable formatted version of the block.
// This is what users see when browsing or displaying search results.
//
// Key patterns:
// - Use emojis and formatting for visual appeal
// - Include the most relevant information
// - Use core.FormatMetadata for consistent metadata display
// - Keep it concise but informative
func (b *TimestampBlock) PrettyText() string {
	metadataInfo := core.FormatMetadata(b.metadata)
	return fmt.Sprintf("🕒 %s\n  Unix: %d\n  Source: %s%s",
		b.timestamp.Format("2006-01-02 15:04:05"),
		b.unix,
		b.source,
		metadataInfo)
}

// Summary returns a concise one-line summary of the timestamp.
func (b *TimestampBlock) Summary() string {
	return fmt.Sprintf("🕒 %s", b.timestamp.Format("2006-01-02 15:04:05"))
}

// Domain-specific accessor methods - these provide type-safe access to block data

// Timestamp returns the time value this block represents
func (b *TimestampBlock) Timestamp() time.Time { return b.timestamp }

// Unix returns the Unix timestamp for easy comparison and sorting
func (b *TimestampBlock) Unix() int64 { return b.unix }

// Factory creates a new TimestampBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (b *TimestampBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	// Extract domain-specific data from metadata using safe helper functions
	metadata := genericBlock.Metadata()
	timestampStr := getStringFromMetadata(metadata, "timestamp", "")
	unix := getInt64FromMetadata(metadata, "unix", 0)

	// Reconstruct the timestamp, preferring the ISO format but falling back to unix
	var timestamp time.Time
	if timestampStr != "" {
		if parsed, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			timestamp = parsed.UTC()
		} else {
			timestamp = time.Unix(unix, 0).UTC()
		}
	} else {
		timestamp = time.Unix(unix, 0).UTC()
	}

	// Reconstruct the block with all original data
	return &TimestampBlock{
		id:        genericBlock.ID(),
		text:      genericBlock.Text(),
		createdAt: genericBlock.CreatedAt(),
		source:    source,   // Use source provided by core system
		metadata:  metadata, // Preserve original metadata
		timestamp: timestamp,
		unix:      unix,
	}
}

// BlockFactory implements the BlockFactory interface for timestamp blocks.
// This is essential for reconstructing blocks when loading from the database.
//
// The factory pattern allows the system to recreate strongly-typed blocks
// from generic database data, preserving all domain-specific functionality.
type BlockFactory struct{}

// CreateFromGeneric reconstructs a TimestampBlock from database data.
// This method is called when loading blocks from storage.
//
// Key patterns:
// - Extract all domain-specific data from metadata using helper functions
// - Handle missing or invalid data gracefully with defaults
// - Reconstruct the block with original data intact
// - Preserve the source from metadata (critical for data isolation)
//
// Parameters:
//   - id: The block ID from database
//   - text: The searchable text from database
//   - createdAt: When the block was originally created
//   - metadata: All structured data stored in database
func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	// Extract domain-specific data from metadata using safe helper functions
	timestampStr := getStringFromMetadata(metadata, "timestamp", "")
	unix := getInt64FromMetadata(metadata, "unix", 0)

	// Reconstruct the timestamp, preferring the ISO format but falling back to unix
	var timestamp time.Time
	if timestampStr != "" {
		if parsed, err := time.Parse(time.RFC3339, timestampStr); err == nil {
			timestamp = parsed.UTC()
		} else {
			timestamp = time.Unix(unix, 0).UTC()
		}
	} else {
		timestamp = time.Unix(unix, 0).UTC()
	}

	// Reconstruct the block with all original data
	return &TimestampBlock{
		id:        id,
		text:      text,
		createdAt: createdAt,
		source:    source,   // Use source provided by core system
		metadata:  metadata, // Preserve original metadata
		timestamp: timestamp,
		unix:      unix,
	}
}

// Helper functions for safe metadata extraction
// These patterns should be used by all datasources to handle type conversion safely

// getStringFromMetadata safely extracts a string value from metadata.
// Returns defaultValue if the key doesn't exist or isn't a string.
// This prevents panics when reconstructing blocks from database data.
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
	if value, exists := metadata[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// getInt64FromMetadata safely extracts an int64 value from metadata.
// Handles multiple numeric types that might be stored in the database.
// Returns defaultValue if the key doesn't exist or isn't a number.
//
// This handles the fact that JSON/database storage might convert numbers
// between different types (int, int64, float64).
func getInt64FromMetadata(metadata map[string]interface{}, key string, defaultValue int64) int64 {
	if value, exists := metadata[key]; exists {
		switch v := value.(type) {
		case int64:
			return v
		case int:
			return int64(v)
		case float64:
			return int64(v) // Handle JSON number conversion
		}
	}
	return defaultValue
}
