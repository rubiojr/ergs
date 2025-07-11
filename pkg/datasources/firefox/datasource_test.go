package firefox

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	_ "github.com/mattn/go-sqlite3"
	"github.com/rubiojr/ergs/pkg/core"
)

func TestFirefoxDatasourceBasicFunctionality(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_places.sqlite")

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
	ds, err := NewDatasource("test-firefox", config)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	// Test datasource properties
	if ds.Name() != "test-firefox" {
		t.Errorf("Expected name 'test-firefox', got '%s'", ds.Name())
	}

	if ds.Type() != "firefox" {
		t.Errorf("Expected type 'firefox', got '%s'", ds.Type())
	}

	schema := ds.Schema()
	expectedFields := []string{"url", "title", "description", "visit_date"}
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

func TestFirefoxDataFetching(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_places.sqlite")

	// Create test database with sample data
	if err := createTestDatabase(dbPath); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := &Config{DatabasePath: dbPath}
	ds, err := NewDatasource("test-firefox", config)
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
		if block.Source() != "test-firefox" {
			t.Errorf("Expected source 'test-firefox', got '%s'", block.Source())
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
		"url":         "https://example.com",
		"title":       "Example Title",
		"description": "Example Description",
	}

	block := factory.CreateFromGeneric("test-id", "test text", time.Now(), "firefox", metadata)

	if block.ID() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", block.ID())
	}

	if block.Source() != "firefox" {
		t.Errorf("Expected source 'firefox', got '%s'", block.Source())
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

func createTestDatabase(dbPath string) error {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return err
	}
	defer db.Close()

	// Create tables
	createTables := []string{
		`CREATE TABLE moz_places (
			id INTEGER PRIMARY KEY,
			url TEXT UNIQUE,
			title TEXT,
			description TEXT
		)`,
		`CREATE TABLE moz_historyvisits (
			id INTEGER PRIMARY KEY,
			place_id INTEGER,
			visit_date INTEGER,
			FOREIGN KEY(place_id) REFERENCES moz_places(id)
		)`,
	}

	for _, sql := range createTables {
		if _, err := db.Exec(sql); err != nil {
			return err
		}
	}

	// Insert test data
	testData := []struct {
		url, title, description string
		visitDate               int64
	}{
		{
			url:         "https://example.com",
			title:       "Example Domain",
			description: "This domain is for use in examples",
			visitDate:   time.Now().UnixNano() / 1000, // microseconds
		},
		{
			url:         "https://github.com",
			title:       "GitHub",
			description: "Where the world builds software",
			visitDate:   time.Now().Add(-time.Hour).UnixNano() / 1000,
		},
	}

	for i, data := range testData {
		placeID := i + 1

		// Insert place
		_, err := db.Exec(
			"INSERT INTO moz_places (id, url, title, description) VALUES (?, ?, ?, ?)",
			placeID, data.url, data.title, data.description,
		)
		if err != nil {
			return err
		}

		// Insert visit
		_, err = db.Exec(
			"INSERT INTO moz_historyvisits (place_id, visit_date) VALUES (?, ?)",
			placeID, data.visitDate,
		)
		if err != nil {
			return err
		}
	}

	return nil
}
