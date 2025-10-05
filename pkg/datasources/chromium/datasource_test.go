package chromium

import (
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

func TestChromiumDatasourceBasicFunctionality(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_history.db")

	// Create test database
	if err := createTestDatabase(dbPath); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Test config validation
	config := &Config{DatabasePath: dbPath}
	if err := config.Validate(); err != nil {
		t.Errorf("Valid config should not return error: %v", err)
	}

	// Test invalid config
	invalidConfig := &Config{DatabasePath: "/nonexistent/path"}
	if err := invalidConfig.Validate(); err == nil {
		t.Error("Invalid config should return error")
	}

	// Test datasource creation
	ds, err := NewDatasource("test-chromium", config)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	// Test datasource properties
	if ds.Name() != "test-chromium" {
		t.Errorf("Expected name 'test-chromium', got '%s'", ds.Name())
	}

	if ds.Type() != "chromium" {
		t.Errorf("Expected type 'chromium', got '%s'", ds.Type())
	}

	schema := ds.Schema()
	expectedFields := []string{"url", "title", "visit_date"}
	for _, field := range expectedFields {
		if _, exists := schema[field]; !exists {
			t.Errorf("Schema missing expected field: %s", field)
		}
	}

	// Test block prototype
	prototype := ds.BlockPrototype()
	if prototype == nil {
		t.Error("Block prototype should not be nil")
	}
}

func TestChromiumDataFetching(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_history.db")

	// Create test database with sample data
	if err := createTestDatabase(dbPath); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := &Config{DatabasePath: dbPath}
	ds, err := NewDatasource("test-chromium", config)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	ctx := context.Background()
	blockCh := make(chan core.Block, 10)

	// Fetch blocks in a goroutine
	go func() {
		defer close(blockCh)
		if err := ds.FetchBlocks(ctx, blockCh); err != nil {
			t.Errorf("FetchBlocks failed: %v", err)
		}
	}()

	// Collect blocks
	var blocks []core.Block
	for block := range blockCh {
		blocks = append(blocks, block)
	}

	// Verify we got the expected number of blocks
	if len(blocks) != 2 {
		t.Errorf("Expected 2 blocks, got %d", len(blocks))
	}

	// Verify block properties
	for _, block := range blocks {
		if block.Source() != "test-chromium" {
			t.Errorf("Expected source 'test-chromium', got '%s'", block.Source())
		}

		if block.ID() == "" {
			t.Error("Block ID should not be empty")
		}

		metadata := block.Metadata()
		if url, exists := metadata["url"]; !exists || url == "" {
			t.Error("Block should have non-empty URL in metadata")
		}
	}
}

func TestBlockFactory(t *testing.T) {
	factory := &BlockFactory{}

	metadata := map[string]interface{}{
		"url":   "https://example.com",
		"title": "Example Title",
	}

	block := factory.CreateFromGeneric("test-id", "test text", time.Now(), "chromium", metadata)

	if block.ID() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", block.ID())
	}

	if block.Source() != "chromium" {
		t.Errorf("Expected source 'chromium', got '%s'", block.Source())
	}

	visitBlock, ok := block.(*VisitBlock)
	if !ok {
		t.Fatal("Block should be of type *VisitBlock")
	}

	if visitBlock.URL() != "https://example.com" {
		t.Errorf("Expected URL 'https://example.com', got '%s'", visitBlock.URL())
	}

	if visitBlock.Title() != "Example Title" {
		t.Errorf("Expected title 'Example Title', got '%s'", visitBlock.Title())
	}
}

func TestChromeTimeConversion(t *testing.T) {
	// Test known Chrome timestamp
	// 13404138406719720 microseconds since 1601-01-01
	chromeTime := int64(13404138406719720)

	result := chromeTimeToUnix(chromeTime)

	// Verify it's a reasonable time (should be in 2024)
	if result.Year() < 2020 || result.Year() > 2030 {
		t.Errorf("Expected year between 2020-2030, got %d", result.Year())
	}
}

func createTestDatabase(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Printf("Warning: failed to close database: %v\n", err)
		}
	}()

	// Create tables matching Chromium schema
	createTables := []string{
		`CREATE TABLE urls (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url LONGVARCHAR,
			title LONGVARCHAR,
			visit_count INTEGER DEFAULT 0 NOT NULL,
			typed_count INTEGER DEFAULT 0 NOT NULL,
			last_visit_time INTEGER NOT NULL,
			hidden INTEGER DEFAULT 0 NOT NULL
		)`,
		`CREATE TABLE visits (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			url INTEGER NOT NULL,
			visit_time INTEGER NOT NULL,
			from_visit INTEGER,
			transition INTEGER DEFAULT 0 NOT NULL,
			segment_id INTEGER,
			visit_duration INTEGER DEFAULT 0 NOT NULL
		)`,
	}

	for _, sql := range createTables {
		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	// Insert test data
	// Chrome timestamps are in microseconds since 1601-01-01 UTC
	// We'll use a recent timestamp
	const chromeEpochOffset = 11644473600
	now := time.Now()
	hourAgo := now.Add(-time.Hour)

	nowChromeTime := (now.Unix() + chromeEpochOffset) * 1000000
	hourAgoChromeTime := (hourAgo.Unix() + chromeEpochOffset) * 1000000

	testData := []struct {
		url       string
		title     string
		visitTime int64
	}{
		{
			url:       "https://example.com",
			title:     "Example Domain",
			visitTime: nowChromeTime,
		},
		{
			url:       "https://github.com",
			title:     "GitHub",
			visitTime: hourAgoChromeTime,
		},
	}

	for i, data := range testData {
		urlID := i + 1

		// Insert URL
		_, err := db.Exec(
			"INSERT INTO urls (id, url, title, visit_count, last_visit_time, hidden) VALUES (?, ?, ?, ?, ?, ?)",
			urlID, data.url, data.title, 1, data.visitTime, 0,
		)
		if err != nil {
			return err
		}

		// Insert visit
		_, err = db.Exec(
			"INSERT INTO visits (url, visit_time, transition) VALUES (?, ?, ?)",
			urlID, data.visitTime, 0,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
