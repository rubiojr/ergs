// Package rtve provides block implementation for RTVE datasource.
// This file demonstrates block patterns for RTVE TV show episodes:
// - Block struct with video metadata
// - Constructor functions with proper metadata handling
// - Factory for database reconstruction
// - Helper functions for safe type conversion
package rtve

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// VTTCue represents a single subtitle cue with timing and text
type VTTCue struct {
	StartTime string `json:"start"`
	EndTime   string `json:"end"`
	Text      string `json:"text"`
}

// RTVEBlock represents a single RTVE video/episode entry in the system.
// This block stores metadata about RTVE TV show episodes including
// title, publication date, URLs, and subtitle availability.
type RTVEBlock struct {
	// Core Block interface fields - required for all blocks
	id        string                 // Unique identifier for this block
	text      string                 // Searchable text content
	createdAt time.Time              // When this block was created
	source    string                 // Source datasource instance name
	metadata  map[string]interface{} // Structured data for database storage

	// Domain-specific fields for RTVE videos
	videoID         string    // RTVE video ID
	longTitle       string    // Full title of the episode
	publicationDate string    // Publication date in RTVE format
	htmlURL         string    // URL to watch the video
	uri             string    // RTVE API URI
	hasSubtitles    bool      // Whether subtitles are available
	subtitleLangs   []string  // Available subtitle languages
	subtitleText    string    // Spanish subtitle text content (for search)
	publishedAt     time.Time // Parsed publication date
}

// NewRTVEBlock creates a new RTVE block with default source.
// This is a convenience constructor for backward compatibility.
func NewRTVEBlock(videoID, longTitle, publicationDate, htmlURL, uri string, hasSubtitles bool, subtitleLangs []string, subtitleText string) *RTVEBlock {
	return NewRTVEBlockWithSource(videoID, longTitle, publicationDate, htmlURL, uri, hasSubtitles, subtitleLangs, subtitleText, "rtve")
}

// NewRTVEBlockWithSource creates a new RTVE block with explicit source.
// This is the preferred constructor that properly handles data isolation.
//
// Parameters:
//   - videoID: The RTVE video identifier
//   - longTitle: Full title of the episode
//   - publicationDate: Publication date string from RTVE API
//   - htmlURL: URL to watch the video
//   - uri: RTVE API URI
//   - hasSubtitles: Whether subtitles are available
//   - subtitleLangs: List of available subtitle languages
//   - subtitleText: Spanish subtitle text content (empty if not available)
//   - source: The datasource instance name (for proper data isolation)
func NewRTVEBlockWithSource(videoID, longTitle, publicationDate, htmlURL, uri string, hasSubtitles bool, subtitleLangs []string, subtitleText, source string) *RTVEBlock {
	// Parse publication date - RTVE uses format: "02-01-2006 15:04:05" in Europe/Madrid timezone
	const rtveLayout = "02-01-2006 15:04:05"
	var publishedAt time.Time

	// Load Europe/Madrid timezone for correct parsing
	madridLoc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		// Fallback to UTC if timezone loading fails
		madridLoc = time.UTC
	}

	if parsed, err := time.ParseInLocation(rtveLayout, publicationDate, madridLoc); err == nil {
		publishedAt = parsed.UTC() // Convert to UTC for storage
	} else {
		// Fallback to current time if parsing fails
		publishedAt = time.Now().UTC()
	}

	// Generate searchable text - include all relevant terms for full-text search
	textParts := []string{
		longTitle,
		videoID,
	}
	if len(subtitleLangs) > 0 {
		textParts = append(textParts, strings.Join(subtitleLangs, " "))
	}
	// Include subtitle text for full-text search (extract text from JSON cues)
	if subtitleText != "" {
		var cues []VTTCue
		if err := json.Unmarshal([]byte(subtitleText), &cues); err == nil {
			for _, cue := range cues {
				textParts = append(textParts, cue.Text)
			}
		}
	}
	text := strings.Join(textParts, " ")

	// Metadata contains all structured data needed for database storage
	metadata := map[string]interface{}{
		"video_id":         videoID,
		"long_title":       longTitle,
		"publication_date": publicationDate,
		"html_url":         htmlURL,
		"uri":              uri,
		"has_subtitles":    hasSubtitles,
		"subtitle_langs":   strings.Join(subtitleLangs, ","),
		"subtitle_text":    subtitleText,
		"source":           source,
	}

	// Block ID should be unique and deterministic
	// Using video ID ensures uniqueness and allows deduplication
	blockID := fmt.Sprintf("rtve-%s", videoID)

	return &RTVEBlock{
		id:              blockID,
		text:            text,
		createdAt:       publishedAt,
		source:          source,
		metadata:        metadata,
		videoID:         videoID,
		longTitle:       longTitle,
		publicationDate: publicationDate,
		htmlURL:         htmlURL,
		uri:             uri,
		hasSubtitles:    hasSubtitles,
		subtitleLangs:   subtitleLangs,
		subtitleText:    subtitleText,
		publishedAt:     publishedAt,
	}
}

// Core Block interface implementation - these methods are required for all blocks

// ID returns the unique identifier for this block
func (b *RTVEBlock) ID() string { return b.id }

// Text returns the searchable text content
func (b *RTVEBlock) Text() string { return b.text }

// CreatedAt returns when this block was created (publication date)
func (b *RTVEBlock) CreatedAt() time.Time { return b.createdAt }

// Source returns the datasource instance name that created this block
func (b *RTVEBlock) Source() string { return b.source }

// Metadata returns structured data for database storage and reconstruction
func (b *RTVEBlock) Metadata() map[string]interface{} { return b.metadata }

// Type returns the block type identifier
func (b *RTVEBlock) Type() string {
	return "rtve"
}

// PrettyText returns a human-readable formatted version of the block.
// This is what users see when browsing or displaying search results.
func (b *RTVEBlock) PrettyText() string {
	subtitlesInfo := "No"
	if b.hasSubtitles && len(b.subtitleLangs) > 0 {
		subtitlesInfo = fmt.Sprintf("Yes (%s)", strings.Join(b.subtitleLangs, ", "))
	} else if b.hasSubtitles {
		subtitlesInfo = "Yes"
	}

	metadataInfo := core.FormatMetadata(b.metadata)
	return fmt.Sprintf("📺 %s\n  ID: %s\n  Published: %s\n  Subtitles: %s\n  URL: %s%s",
		b.longTitle,
		b.videoID,
		b.publishedAt.Format("2006-01-02 15:04:05"),
		subtitlesInfo,
		b.htmlURL,
		metadataInfo)
}

// Summary returns a concise one-line summary of the video.
func (b *RTVEBlock) Summary() string {
	return fmt.Sprintf("📺 %s (%s)", b.longTitle, b.publishedAt.Format("2006-01-02"))
}

// Domain-specific accessor methods - these provide type-safe access to block data

// VideoID returns the RTVE video identifier
func (b *RTVEBlock) VideoID() string { return b.videoID }

// LongTitle returns the full title of the episode
func (b *RTVEBlock) LongTitle() string { return b.longTitle }

// PublicationDate returns the original publication date string from RTVE
func (b *RTVEBlock) PublicationDate() string { return b.publicationDate }

// HTMLURL returns the URL to watch the video
func (b *RTVEBlock) HTMLURL() string { return b.htmlURL }

// URI returns the RTVE API URI
func (b *RTVEBlock) URI() string { return b.uri }

// HasSubtitles returns whether subtitles are available
func (b *RTVEBlock) HasSubtitles() bool { return b.hasSubtitles }

// SubtitleLangs returns the list of available subtitle languages
func (b *RTVEBlock) SubtitleLangs() []string { return b.subtitleLangs }

// SubtitleText returns the Spanish subtitle text content
func (b *RTVEBlock) SubtitleText() string { return b.subtitleText }

// PublishedAt returns the parsed publication date
func (b *RTVEBlock) PublishedAt() time.Time { return b.publishedAt }

// Factory creates a new RTVEBlock from a GenericBlock and source.
// This method is part of the core.Block interface and enables reconstruction
// from database data without requiring separate factory objects.
func (b *RTVEBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	// Extract domain-specific data from metadata using safe helper functions
	metadata := genericBlock.Metadata()
	videoID := getStringFromMetadata(metadata, "video_id", "")
	longTitle := getStringFromMetadata(metadata, "long_title", "")
	publicationDate := getStringFromMetadata(metadata, "publication_date", "")
	htmlURL := getStringFromMetadata(metadata, "html_url", "")
	uri := getStringFromMetadata(metadata, "uri", "")
	hasSubtitles := getBoolFromMetadata(metadata, "has_subtitles", false)
	subtitleLangsStr := getStringFromMetadata(metadata, "subtitle_langs", "")
	subtitleText := getStringFromMetadata(metadata, "subtitle_text", "")

	// Parse subtitle languages
	var subtitleLangs []string
	if subtitleLangsStr != "" {
		subtitleLangs = strings.Split(subtitleLangsStr, ",")
	}

	// Parse publication date
	const rtveLayout = "02-01-2006 15:04:05"
	var publishedAt time.Time

	// Load Europe/Madrid timezone for correct parsing
	madridLoc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		madridLoc = time.UTC
	}

	if publicationDate != "" {
		if parsed, err := time.ParseInLocation(rtveLayout, publicationDate, madridLoc); err == nil {
			publishedAt = parsed.UTC() // Convert to UTC for storage
		} else {
			publishedAt = genericBlock.CreatedAt()
		}
	} else {
		publishedAt = genericBlock.CreatedAt()
	}

	// Reconstruct the block with all original data
	return &RTVEBlock{
		id:              genericBlock.ID(),
		text:            genericBlock.Text(),
		createdAt:       genericBlock.CreatedAt(),
		source:          source,
		metadata:        metadata,
		videoID:         videoID,
		longTitle:       longTitle,
		publicationDate: publicationDate,
		htmlURL:         htmlURL,
		uri:             uri,
		hasSubtitles:    hasSubtitles,
		subtitleLangs:   subtitleLangs,
		subtitleText:    subtitleText,
		publishedAt:     publishedAt,
	}
}

// BlockFactory implements the BlockFactory interface for RTVE blocks.
// This is essential for reconstructing blocks when loading from the database.
type BlockFactory struct{}

// CreateFromGeneric reconstructs an RTVEBlock from database data.
// This method is called when loading blocks from storage.
func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	// Extract domain-specific data from metadata using safe helper functions
	videoID := getStringFromMetadata(metadata, "video_id", "")
	longTitle := getStringFromMetadata(metadata, "long_title", "")
	publicationDate := getStringFromMetadata(metadata, "publication_date", "")
	htmlURL := getStringFromMetadata(metadata, "html_url", "")
	uri := getStringFromMetadata(metadata, "uri", "")
	hasSubtitles := getBoolFromMetadata(metadata, "has_subtitles", false)
	subtitleLangsStr := getStringFromMetadata(metadata, "subtitle_langs", "")
	subtitleText := getStringFromMetadata(metadata, "subtitle_text", "")

	// Parse subtitle languages
	var subtitleLangs []string
	if subtitleLangsStr != "" {
		subtitleLangs = strings.Split(subtitleLangsStr, ",")
	}

	// Parse publication date
	const rtveLayout = "02-01-2006 15:04:05"
	var publishedAt time.Time

	// Load Europe/Madrid timezone for correct parsing
	madridLoc, err := time.LoadLocation("Europe/Madrid")
	if err != nil {
		madridLoc = time.UTC
	}

	if publicationDate != "" {
		if parsed, err := time.ParseInLocation(rtveLayout, publicationDate, madridLoc); err == nil {
			publishedAt = parsed.UTC() // Convert to UTC for storage
		} else {
			publishedAt = createdAt
		}
	} else {
		publishedAt = createdAt
	}

	// Reconstruct the block with all original data
	return &RTVEBlock{
		id:              id,
		text:            text,
		createdAt:       createdAt,
		source:          source,
		metadata:        metadata,
		videoID:         videoID,
		longTitle:       longTitle,
		publicationDate: publicationDate,
		htmlURL:         htmlURL,
		uri:             uri,
		hasSubtitles:    hasSubtitles,
		subtitleLangs:   subtitleLangs,
		subtitleText:    subtitleText,
		publishedAt:     publishedAt,
	}
}

// Helper functions for safe metadata extraction
// These patterns should be used by all datasources to handle type conversion safely

// getStringFromMetadata safely extracts a string value from metadata.
// Returns defaultValue if the key doesn't exist or isn't a string.
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
	if value, exists := metadata[key]; exists {
		if str, ok := value.(string); ok {
			return str
		}
	}
	return defaultValue
}

// getBoolFromMetadata safely extracts a boolean value from metadata.
// Returns defaultValue if the key doesn't exist or isn't a boolean.
func getBoolFromMetadata(metadata map[string]interface{}, key string, defaultValue bool) bool {
	if value, exists := metadata[key]; exists {
		if b, ok := value.(bool); ok {
			return b
		}
		// Handle potential integer representation (0/1)
		if i, ok := value.(int64); ok {
			return i != 0
		}
		if i, ok := value.(int); ok {
			return i != 0
		}
	}
	return defaultValue
}
