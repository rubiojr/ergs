package types

import "time"

// PageData represents data passed to templates
type PageData struct {
	Title               string
	Query               string
	Datasource          string   // For individual datasource pages
	SelectedDatasources []string // For search with multiple datasource selection
	Results             map[string][]WebBlock
	FirehoseBlocks      []WebBlock // Flat, globally time-ordered slice for firehose rendering
	Datasources         []DatasourceInfo
	TotalCount          int
	Error               string
	Success             string
	CurrentPage         int
	HasNextPage         bool
	PageSize            int
	TotalPages          int
	TotalBlocks         int
	ActiveDatasources   int
	OldestBlock         *time.Time
	NewestBlock         *time.Time
	StartDate           *time.Time
	EndDate             *time.Time
	Version             string // Application version (for footer display)
}

// DatasourceInfo represents datasource information
type DatasourceInfo struct {
	Name   string                 `json:"name"`
	Type   string                 `json:"type"`
	Config map[string]interface{} `json:"config,omitempty"`
	Stats  map[string]interface{} `json:"stats,omitempty"`
}

// WebBlock represents a block for web display
type WebBlock struct {
	ID            string
	Text          string
	Source        string
	CreatedAt     time.Time
	Metadata      map[string]interface{}
	Links         []string
	FormattedText string
}
