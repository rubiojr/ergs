// Package rtve provides a datasource implementation for fetching RTVE (Radio Televisión Española)
// TV show episodes and their metadata using the rtve-go library.
//
// This datasource allows fetching the latest episodes from RTVE shows by show ID,
// with configurable limits on the number of episodes to retrieve.
//
// Features:
// - Fetches latest episodes for configured RTVE shows
// - Includes video metadata (title, publication date, URLs)
// - Captures subtitle availability and languages
// - Configurable maximum number of episodes per show
// - Self-registration via init() function
//
// Example configuration:
//
//	[datasources.my_rtve]
//	type = 'rtve'
//	interval = '1h0m0s'
//	[datasources.my_rtve.config]
//	show_id = 'telediario-1'
//	max_episodes = 10
package rtve

import (
	"context"
	"fmt"
	"log"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/rtve-go/api"
)

// init registers this datasource with the core system.
// This is called automatically when the package is imported.
func init() {
	// Register a prototype instance for factory creation
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("rtve", prototype)
}

// Config defines the configuration structure for the RTVE datasource.
type Config struct {
	// ShowID is the RTVE show identifier (e.g., "telediario-1", "telediario-2", "informe-semanal")
	// Use api.AvailableShows() to see all available shows
	ShowID string `toml:"show_id"`

	// MaxEpisodes is the maximum number of latest episodes to fetch per run
	// Defaults to 10 if not specified or <= 0
	MaxEpisodes int `toml:"max_episodes"`
}

// Validate ensures the configuration is valid and sets defaults.
func (c *Config) Validate() error {
	if c.ShowID == "" {
		return fmt.Errorf("show_id is required")
	}

	// Validate show ID against available shows
	availableShows := api.AvailableShows()
	validShow := false
	for _, show := range availableShows {
		if show == c.ShowID {
			validShow = true
			break
		}
	}
	if !validShow {
		return fmt.Errorf("invalid show_id: %s (available shows: %v)", c.ShowID, availableShows)
	}

	// Set default max episodes if not specified
	if c.MaxEpisodes <= 0 {
		c.MaxEpisodes = 10
	}

	// Cap at reasonable maximum to prevent excessive fetching
	if c.MaxEpisodes > 100 {
		c.MaxEpisodes = 100
	}

	return nil
}

// Datasource implements the core.Datasource interface for RTVE TV shows.
type Datasource struct {
	config       *Config // Configuration specific to this datasource
	instanceName string  // The unique name for this datasource instance
}

// NewDatasource creates a new RTVE datasource instance.
//
// Parameters:
//   - instanceName: The unique identifier for this datasource instance
//   - config: Configuration object (can be nil for defaults)
//
// Returns the configured datasource or an error if configuration is invalid.
func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var rtveConfig *Config

	// Handle nil config by providing sensible defaults
	if config == nil {
		rtveConfig = &Config{
			ShowID:      "telediario-1", // Default to Telediario 1
			MaxEpisodes: 10,
		}
	} else {
		// Type assertion to ensure we have the correct config type
		var ok bool
		rtveConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for RTVE datasource")
		}
	}

	// Validate configuration
	if err := rtveConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid RTVE configuration: %w", err)
	}

	return &Datasource{
		config:       rtveConfig,
		instanceName: instanceName,
	}, nil
}

// Type returns the datasource type identifier.
func (d *Datasource) Type() string {
	return "rtve"
}

// Name returns the instance name for this datasource.
func (d *Datasource) Name() string {
	return d.instanceName
}

// Schema defines the database schema for blocks created by this datasource.
func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"video_id":         "TEXT",    // RTVE video identifier
		"long_title":       "TEXT",    // Full episode title
		"publication_date": "TEXT",    // Publication date string
		"html_url":         "TEXT",    // URL to watch the video
		"uri":              "TEXT",    // RTVE API URI
		"has_subtitles":    "INTEGER", // Boolean: whether subtitles are available
		"subtitle_langs":   "TEXT",    // Comma-separated list of subtitle languages
	}
}

// BlockPrototype returns a prototype block used for reconstruction from database data.
func (d *Datasource) BlockPrototype() core.Block {
	return &RTVEBlock{}
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
	return fmt.Errorf("invalid config type for RTVE datasource")
}

// GetConfig returns the current configuration.
func (d *Datasource) GetConfig() interface{} {
	return d.config
}

// FetchBlocks fetches the latest episodes for the configured RTVE show.
// It uses the rtve-go library's FetchShowLatest function to retrieve episodes
// and streams them as blocks through the provided channel.
//
// Parameters:
//   - ctx: Context for cancellation and timeouts
//   - blockCh: Channel to send created blocks through
func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("RTVE: Fetching latest %d episodes from show '%s'", d.config.MaxEpisodes, d.config.ShowID)

	// Create a visitor function that processes each video result
	// and sends it as a block through the channel
	visitor := func(result *api.VideoResult) error {
		// Check for context cancellation
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Extract subtitle information
		hasSubtitles := false
		var subtitleLangs []string
		if result.Subtitles != nil && len(result.Subtitles.Subtitles) > 0 {
			hasSubtitles = true
			for _, sub := range result.Subtitles.Subtitles {
				subtitleLangs = append(subtitleLangs, sub.Lang)
			}
		}

		// Create a block from the video metadata
		block := NewRTVEBlockWithSource(
			result.Metadata.ID,
			result.Metadata.LongTitle,
			result.Metadata.PublicationDate,
			result.Metadata.HTMLUrl,
			result.Metadata.URI,
			hasSubtitles,
			subtitleLangs,
			d.instanceName, // Use instance name for proper data isolation
		)

		// Send block through channel
		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
			log.Printf("RTVE: Fetched episode '%s' (ID: %s)", result.Metadata.LongTitle, result.Metadata.ID)
			return nil
		}
	}

	// Fetch the latest episodes using the rtve-go API
	stats, err := api.FetchShowLatest(d.config.ShowID, d.config.MaxEpisodes, visitor)
	if err != nil {
		return fmt.Errorf("error fetching RTVE show '%s': %w", d.config.ShowID, err)
	}

	log.Printf("RTVE: Successfully fetched %d episodes from show '%s' (%d errors)",
		stats.VideosProcessed, d.config.ShowID, stats.ErrorCount)

	// Log any non-fatal errors that occurred during fetching
	if stats.ErrorCount > 0 && len(stats.Errors) > 0 {
		log.Printf("RTVE: Encountered %d non-fatal errors:", stats.ErrorCount)
		for i, err := range stats.Errors {
			if i < 5 { // Limit to first 5 errors to avoid spam
				log.Printf("  - %v", err)
			}
		}
		if len(stats.Errors) > 5 {
			log.Printf("  ... and %d more errors", len(stats.Errors)-5)
		}
	}

	return nil
}

// Close performs cleanup when the datasource is no longer needed.
func (d *Datasource) Close() error {
	return nil
}

// Factory creates a new instance of this datasource.
// This method is part of the core.Datasource interface and is called
// by the core system when creating datasource instances.
func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
