package zedthreads

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/klauspost/compress/zstd"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

func TestZedThreadsDatasourceBasicFunctionality(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_threads.db")

	// Create test database
	if err := createTestDatabase(dbPath); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	// Test config validation
	config := &Config{DatabasePath: dbPath}
	if err := config.Validate(); err != nil {
		t.Errorf("Valid config should not return error: %v", err)
	}

	// Test default config (empty path should use default)
	defaultConfig := &Config{}
	if err := defaultConfig.Validate(); err != nil {
		// Only error if the default path doesn't exist, which is expected in test environment
		if defaultConfig.DatabasePath == "" {
			t.Errorf("Default config should set database path, but got empty string")
		}
		// Don't fail the test if default path doesn't exist - that's expected in CI
	}

	// Test invalid config
	invalidConfig := &Config{DatabasePath: "/nonexistent/path"}
	if err := invalidConfig.Validate(); err == nil {
		t.Error("Invalid config should return error")
	}

	// Test datasource creation
	ds, err := NewDatasource("test-zedthreads", config)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}
	defer func() {
		if err := ds.Close(); err != nil {
			t.Logf("Warning: failed to close datasource: %v", err)
		}
	}()

	// Test datasource properties
	if ds.Name() != "test-zedthreads" {
		t.Errorf("Expected name 'test-zedthreads', got '%s'", ds.Name())
	}

	if ds.Type() != "zedthreads" {
		t.Errorf("Expected type 'zedthreads', got '%s'", ds.Type())
	}

	schema := ds.Schema()
	expectedFields := []string{"summary", "updated_at", "model", "version", "message_count", "user_messages", "assistant_messages"}
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

func TestZedThreadsDataFetching(t *testing.T) {
	// Create a temporary database for testing
	tempDir := t.TempDir()
	dbPath := filepath.Join(tempDir, "test_threads.db")

	// Create test database with sample data
	if err := createTestDatabase(dbPath); err != nil {
		t.Fatalf("Failed to create test database: %v", err)
	}

	config := &Config{DatabasePath: dbPath}
	ds, err := NewDatasource("test-zedthreads", config)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}
	defer func() {
		if err := ds.Close(); err != nil {
			t.Logf("Warning: failed to close datasource: %v", err)
		}
	}()

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
		if block.Source() != "test-zedthreads" {
			t.Errorf("Expected source 'test-zedthreads', got '%s'", block.Source())
		}

		if block.ID() == "" {
			t.Error("Block ID should not be empty")
		}

		metadata := block.Metadata()
		if summary, exists := metadata["summary"]; !exists || summary == "" {
			t.Error("Block should have non-empty summary in metadata")
		}

		// Test specific properties from metadata
		if summary, exists := metadata["summary"]; exists {
			if summaryStr, ok := summary.(string); ok && summaryStr == "" {
				t.Error("Thread block should have non-empty summary")
			}
		}
	}
}

func TestBlockFactory(t *testing.T) {
	factory := &BlockFactory{}

	metadata := map[string]interface{}{
		"summary":            "Test Thread",
		"model":              "gpt-4",
		"version":            "0.2.0",
		"message_count":      3,
		"user_messages":      2,
		"assistant_messages": 1,
		"token_total":        150,
	}

	block := factory.CreateFromGeneric("test-id", "test text", time.Now(), "zedthreads", metadata)

	if block.ID() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", block.ID())
	}

	if block.Source() != "zedthreads" {
		t.Errorf("Expected source 'zedthreads', got '%s'", block.Source())
	}

	// Verify metadata contains expected data
	blockMetadata := block.Metadata()
	if summary, exists := blockMetadata["summary"]; !exists || summary != "Test Thread" {
		t.Errorf("Expected summary 'Test Thread' in metadata")
	}
}

func TestThreadBlockMethods(t *testing.T) {
	messages := []Message{
		{
			ID:   0,
			Role: "user",
			Segments: []Segment{
				{Type: "text", Text: "Hello, this is a test question."},
			},
			IsHidden: false,
		},
		{
			ID:   1,
			Role: "assistant",
			Segments: []Segment{
				{Type: "text", Text: "Hello! I can help you with that."},
			},
			IsHidden: false,
		},
		{
			ID:   2,
			Role: "user",
			Segments: []Segment{
				{Type: "text", Text: "Thank you for the help."},
			},
			IsHidden: false,
		},
	}

	threadData := &ThreadData{
		Version:  "0.2.0",
		Summary:  "Test Conversation",
		Model:    &Model{Provider: "openai", Model: "gpt-4"},
		Messages: messages,
		TokenUsage: map[string]interface{}{
			"total":  100,
			"input":  60,
			"output": 40,
		},
	}

	block := NewThreadBlock("test-id", "Test Conversation", time.Now(), threadData)

	// Test message counts
	if block.MessageCount() != 3 {
		t.Errorf("Expected 3 messages, got %d", block.MessageCount())
	}

	if block.UserMessageCount() != 2 {
		t.Errorf("Expected 2 user messages, got %d", block.UserMessageCount())
	}

	if block.AssistantMessageCount() != 1 {
		t.Errorf("Expected 1 assistant message, got %d", block.AssistantMessageCount())
	}

	// Test first user message
	firstMsg := block.FirstUserMessage()
	if firstMsg != "Hello, this is a test question." {
		t.Errorf("Expected first user message, got '%s'", firstMsg)
	}

	// Test pretty text formatting
	prettyText := block.PrettyText()
	if prettyText == "" {
		t.Error("Pretty text should not be empty")
	}

	// Check that it contains expected elements
	expectedElements := []string{"Zed Thread", "Test Conversation", "3 messages", "openai/gpt-4"}
	for _, element := range expectedElements {
		if !contains(prettyText, element) {
			t.Errorf("Pretty text should contain '%s', got: %s", element, prettyText)
		}
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

	// Create the threads table
	createTableSQL := `
	CREATE TABLE threads (
		id TEXT PRIMARY KEY,
		summary TEXT NOT NULL,
		updated_at TEXT NOT NULL,
		data_type TEXT NOT NULL,
		data BLOB NOT NULL
	);`

	if _, err := db.Exec(createTableSQL); err != nil {
		return err
	}

	// Create test thread data
	testThreads := []struct {
		id        string
		summary   string
		updatedAt string
		messages  []Message
	}{
		{
			id:        "thread-1",
			summary:   "How to optimize Go code",
			updatedAt: "2025-01-15T10:00:00Z",
			messages: []Message{
				{
					ID:   0,
					Role: "user",
					Segments: []Segment{
						{Type: "text", Text: "What are some best practices for optimizing Go code performance?"},
					},
					IsHidden: false,
				},
				{
					ID:   1,
					Role: "assistant",
					Segments: []Segment{
						{Type: "text", Text: "Here are some key strategies for optimizing Go code performance: 1. Profile your code using pprof, 2. Minimize allocations, 3. Use sync.Pool for object reuse..."},
					},
					IsHidden: false,
				},
			},
		},
		{
			id:        "thread-2",
			summary:   "Database migration strategies",
			updatedAt: "2025-01-15T11:30:00Z",
			messages: []Message{
				{
					ID:   0,
					Role: "user",
					Segments: []Segment{
						{Type: "text", Text: "What's the best way to handle database migrations in a production environment?"},
					},
					IsHidden: false,
				},
				{
					ID:   1,
					Role: "assistant",
					Segments: []Segment{
						{Type: "text", Text: "For production database migrations, consider these approaches: 1. Blue-green deployments, 2. Rolling migrations, 3. Feature flags..."},
					},
					IsHidden: false,
				},
				{
					ID:   2,
					Role: "user",
					Segments: []Segment{
						{Type: "text", Text: "Can you elaborate on blue-green deployments?"},
					},
					IsHidden: false,
				},
			},
		},
	}

	// Create zstd encoder
	encoder, err := zstd.NewWriter(nil)
	if err != nil {
		return err
	}
	defer func() {
		if err := encoder.Close(); err != nil {
			fmt.Printf("Warning: failed to close encoder: %v\n", err)
		}
	}()

	// Insert test data
	for _, thread := range testThreads {
		threadData := ThreadData{
			Version:   "0.2.0",
			Summary:   thread.summary,
			UpdatedAt: thread.updatedAt,
			Messages:  thread.messages,
			Model:     &Model{Provider: "openai", Model: "gpt-4"},
			TokenUsage: map[string]interface{}{
				"total":  len(thread.messages) * 50,
				"input":  len(thread.messages) * 30,
				"output": len(thread.messages) * 20,
			},
		}

		// Convert to JSON
		jsonData, err := json.Marshal(threadData)
		if err != nil {
			return err
		}

		// Compress with zstd
		compressedData := encoder.EncodeAll(jsonData, nil)

		// Insert into database
		_, err = db.Exec(
			"INSERT INTO threads (id, summary, updated_at, data_type, data) VALUES (?, ?, ?, ?, ?)",
			thread.id, thread.summary, thread.updatedAt, "zstd", compressedData,
		)
		if err != nil {
			return err
		}
	}

	return nil
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || (len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsAt(s, substr))))
}

func containsAt(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
