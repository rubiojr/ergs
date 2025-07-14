package core

import (
	"fmt"
	"strings"
	"time"
)

// Block represents a single unit of data in Ergs with searchable content and metadata.
// All datasources must implement blocks that satisfy this interface.
//
// Blocks are the fundamental data structure in Ergs - they represent indexed, searchable
// content with associated metadata. Each block has a unique identifier, searchable text,
// creation timestamp, source information, and structured metadata for database storage.
//
// Key design principles:
// - Immutable: Once created, blocks should not be modified
// - Self-contained: All necessary data should be accessible through the interface
// - Searchable: Text() should contain all content users might search for
// - Displayable: PrettyText() should provide human-readable formatting
// - Persistable: Metadata() must contain all data needed for database reconstruction
// - Self-constructing: Factory() method allows reconstruction from database data
//
// Example implementation pattern:
//
//	type MyBlock struct {
//		id        string
//		text      string
//		createdAt time.Time
//		source    string
//		metadata  map[string]interface{}
//		// ... domain-specific fields
//	}
//
//	func (b *MyBlock) ID() string { return b.id }
//	func (b *MyBlock) Text() string { return b.text }
//	func (b *MyBlock) Factory(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) Block {
//		return NewMyBlockFromData(id, text, createdAt, source, metadata)
//	}
//	// ... implement other methods
type Block interface {
	// ID returns a unique identifier for this block.
	// Should be deterministic and unique across all blocks from the same datasource.
	// Common patterns: "type-uuid", "type-timestamp", "type-hash"
	ID() string

	// Text returns searchable content for full-text search indexing.
	// This is what users will find when searching. Include all relevant
	// searchable terms but keep it concise and relevant.
	// Example: "John Doe john@example.com software engineer golang python"
	Text() string

	// CreatedAt returns when this block was originally created.
	// Used for sorting and time-based filtering. Should represent
	// the actual creation time of the underlying data, not when
	// the block object was instantiated.
	CreatedAt() time.Time

	// Source returns the datasource instance name that created this block.
	// CRITICAL: This must return the source passed to the factory.
	// The core system automatically handles source in metadata - datasource
	// developers don't need to store or manage this value.
	Source() string

	// Metadata returns structured data for database storage and reconstruction.
	// Should contain domain-specific data only - the core system automatically
	// handles source metadata. Keys should use the same names as the datasource Schema().
	Metadata() map[string]interface{}

	// PrettyText returns a human-readable formatted version for display.
	// This is shown to users in search results and when browsing data.
	// Use emojis, formatting, and clear structure for visual appeal.
	// Include the most important information but keep it scannable.
	PrettyText() string

	// Summary returns a concise one-line summary of the block for compact display.
	// Should include the most important identifying information in a brief format.
	// Examples: "GitHub Issue #123: Fix login bug", "Firefox bookmark: Go documentation"
	// Keep it under 80 characters when possible for better readability.
	Summary() string

	// Factory creates a new instance of this block type from database data.
	// Called when loading blocks from storage to restore full functionality.
	// The core system automatically provides the source parameter - datasource
	// developers don't need to handle source metadata.
	//
	// Parameters:
	//   - genericBlock: The generic block loaded from database with basic data
	//   - source: The datasource instance name (provided by core system)
	//
	// The implementation should:
	// - Extract domain-specific data from genericBlock.Metadata() using safe type conversion
	// - Handle missing or invalid metadata gracefully with sensible defaults
	// - Use the provided source parameter directly
	// - Return a fully functional block with all original capabilities
	Factory(genericBlock *GenericBlock, source string) Block
}

// GenericBlock provides a fallback implementation for cases where
// no specific block type is available. Used internally by the system
// when a datasource's BlockFactory is not available or fails.
//
// This ensures the system can always display and work with blocks
// even if the original datasource is not loaded or has issues.
// However, domain-specific functionality will not be available.
type GenericBlock struct {
	id        string                 // Unique identifier for this block
	text      string                 // Searchable text content
	createdAt time.Time              // When this block was created
	source    string                 // Source datasource instance name
	dsType    string                 // Datasource type (e.g., "github", "firefox")
	metadata  map[string]interface{} // Structured data from database
}

// Block interface implementation for GenericBlock

// ID returns the unique identifier for this block
func (b *GenericBlock) ID() string { return b.id }

// Text returns the searchable text content
func (b *GenericBlock) Text() string { return b.text }

// CreatedAt returns when this block was originally created
func (b *GenericBlock) CreatedAt() time.Time { return b.createdAt }

// Source returns the datasource instance name
func (b *GenericBlock) Source() string { return b.source }

// Metadata returns the structured data for this block
func (b *GenericBlock) Metadata() map[string]interface{} { return b.metadata }

// DSType returns the datasource type for this block
func (b *GenericBlock) DSType() string { return b.dsType }

// PrettyText returns a human-readable formatted version of this generic block.
// Since this is a fallback implementation, it provides basic formatting
// with essential information. Datasource-specific blocks should provide
// more tailored and visually appealing formatting.
func (b *GenericBlock) PrettyText() string {
	metadataInfo := FormatMetadata(b.metadata)
	return fmt.Sprintf("📄 %s\n  ID: %s\n  Time: %s\n  Source: %s\n  Type: %s%s",
		b.text, b.id, b.createdAt.Format("2006-01-02 15:04:05"), b.source, b.dsType, metadataInfo)
}

// Summary returns a concise one-line summary for this generic block.
func (b *GenericBlock) Summary() string {
	// Try to extract clean title from metadata first
	if title, exists := b.metadata["title"]; exists {
		if titleStr, ok := title.(string); ok && titleStr != "" {
			return fmt.Sprintf("📄 %s", titleStr)
		}
	}

	// Try other common title fields
	titleFields := []string{"name", "summary", "repo_name"}
	for _, field := range titleFields {
		if value, exists := b.metadata[field]; exists {
			if valueStr, ok := value.(string); ok && valueStr != "" {
				return fmt.Sprintf("📄 %s", valueStr)
			}
		}
	}

	// Fall back to truncated text without metadata noise
	text := b.text
	// Remove common metadata patterns
	if idx := strings.Index(text, " url="); idx != -1 {
		text = text[:idx]
	}
	if idx := strings.Index(text, " author="); idx != -1 {
		text = text[:idx]
	}
	if idx := strings.Index(text, " type="); idx != -1 {
		text = text[:idx]
	}

	return fmt.Sprintf("📄 %s", text)
}

// Factory creates a new GenericBlock from a GenericBlock and source.
// For GenericBlocks, this simply returns a copy with the provided source.
func (b *GenericBlock) Factory(genericBlock *GenericBlock, source string) Block {
	return &GenericBlock{
		id:        genericBlock.ID(),
		text:      genericBlock.Text(),
		createdAt: genericBlock.CreatedAt(),
		source:    source,
		dsType:    genericBlock.DSType(),
		metadata:  genericBlock.Metadata(),
	}
}

// NewGenericBlock creates a new GenericBlock with the provided data.
// This constructor is used internally by the system when creating
// fallback blocks or when no specific block type is available.
//
// Parameters should follow the same patterns as domain-specific blocks:
// - id: Unique identifier, should be deterministic
// - text: Searchable content for full-text indexing
// - source: Datasource instance name (not type)
// - createdAt: Original creation time of the data
// - metadata: All structured data needed for persistence
func NewGenericBlock(id, text, source, dsType string, createdAt time.Time, metadata map[string]interface{}) *GenericBlock {
	return &GenericBlock{
		id:        id,
		text:      text,
		createdAt: createdAt,
		source:    source,
		dsType:    dsType,
		metadata:  metadata,
	}
}
