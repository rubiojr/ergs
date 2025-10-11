package warehouse

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

type mockDatasource struct {
	name   string
	blocks []core.Block
}

func (m *mockDatasource) Type() string {
	return "mock"
}

func (m *mockDatasource) Name() string {
	return m.name
}

func (m *mockDatasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	for _, block := range m.blocks {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
		}
	}
	return nil
}

func (m *mockDatasource) Schema() map[string]any {
	return map[string]any{
		"text":       "TEXT",
		"created_at": "DATETIME",
		"metadata":   "TEXT",
	}
}

func (m *mockDatasource) ConfigType() interface{} {
	return &mockConfig{}
}

func (m *mockDatasource) SetConfig(config interface{}) error {
	return nil
}

func (m *mockDatasource) GetConfig() interface{} {
	return &mockConfig{}
}

func (m *mockDatasource) Close() error {
	return nil
}

func (m *mockDatasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return &mockDatasource{name: instanceName}, nil
}

func (m *mockDatasource) BlockPrototype() core.Block {
	return &mockBlock{}
}

type mockConfig struct{}

func (c *mockConfig) Validate() error {
	return nil
}

type mockBlock struct {
	id        string
	text      string
	createdAt time.Time
	source    string
	metadata  map[string]interface{}
}

func (b *mockBlock) ID() string                       { return b.id }
func (b *mockBlock) Text() string                     { return b.text }
func (b *mockBlock) CreatedAt() time.Time             { return b.createdAt }
func (b *mockBlock) Source() string                   { return b.source }
func (b *mockBlock) Type() string                     { return "mock" }
func (b *mockBlock) Metadata() map[string]interface{} { return b.metadata }
func (b *mockBlock) PrettyText() string               { return b.text }
func (b *mockBlock) Summary() string                  { return b.text }
func (b *mockBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	return &mockBlock{
		id:        genericBlock.ID(),
		text:      genericBlock.Text(),
		createdAt: genericBlock.CreatedAt(),
		source:    source,
		metadata:  genericBlock.Metadata(),
	}
}

func TestWarehouseStreaming(t *testing.T) {
	storageManager, err := storage.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	config := Config{
		OptimizeInterval: 0,
	}
	wh := NewWarehouse(config, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Logf("Warning: failed to close warehouse: %v", err)
		}
	}()

	now := time.Now()
	testBlocks := []core.Block{
		&mockBlock{
			id:        "block1",
			text:      "Test block 1",
			createdAt: now,
			source:    "test-datasource",
			metadata:  map[string]interface{}{"type": "test"},
		},
		&mockBlock{
			id:        "block2",
			text:      "Test block 2",
			createdAt: now.Add(time.Minute),
			source:    "test-datasource",
			metadata:  map[string]interface{}{"type": "test"},
		},
	}

	mockDS := &mockDatasource{
		name:   "test-datasource",
		blocks: testBlocks,
	}

	err = wh.AddDatasource("test-datasource", mockDS)
	if err != nil {
		t.Fatalf("Failed to add datasource: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	err = wh.FetchOnce(ctx)
	if err != nil {
		t.Fatalf("Failed to fetch from warehouse: %v", err)
	}

	blocks, err := wh.SearchBlocks("test-datasource", "", 10)
	if err != nil {
		t.Fatalf("Failed to search blocks: %v", err)
	}

	if len(blocks) != 2 {
		t.Errorf("Expected 2 blocks, got %d", len(blocks))
	}

	for i, block := range blocks {
		expectedText := testBlocks[len(testBlocks)-1-i].Text()
		if block.Text() != expectedText {
			t.Errorf("Block %d text mismatch. Expected %s, got %s", i, expectedText, block.Text())
		}
	}
}

func TestWarehouseStreamingCallback(t *testing.T) {
	storageManager, err := storage.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	config := Config{
		OptimizeInterval: 0,
	}
	wh := NewWarehouse(config, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Logf("Warning: failed to close warehouse: %v", err)
		}
	}()

	now := time.Now()
	testBlocks := []core.Block{
		&mockBlock{
			id:        "stream1",
			text:      "Stream test block 1",
			createdAt: now,
			source:    "test-datasource",
			metadata:  map[string]interface{}{"type": "stream"},
		},
		&mockBlock{
			id:        "stream2",
			text:      "Stream test block 2",
			createdAt: now.Add(time.Minute),
			source:    "test-datasource",
			metadata:  map[string]interface{}{"type": "stream"},
		},
	}

	mockDS := &mockDatasource{
		name:   "test-datasource",
		blocks: testBlocks,
	}

	err = wh.AddDatasource("test-datasource", mockDS)
	if err != nil {
		t.Fatalf("Failed to add datasource: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Track blocks received via streaming callback
	var streamedBlocks []core.Block
	streamingCallback := func(block core.Block) {
		streamedBlocks = append(streamedBlocks, block)
	}

	err = wh.FetchOnce(ctx, WithStreaming(streamingCallback))
	if err != nil {
		t.Fatalf("Failed to fetch with streaming: %v", err)
	}

	// Verify streaming callback received blocks
	if len(streamedBlocks) != 2 {
		t.Errorf("Expected 2 streamed blocks, got %d", len(streamedBlocks))
	}

	// Verify blocks were also stored in database
	blocks, err := wh.SearchBlocks("test-datasource", "", 10)
	if err != nil {
		t.Fatalf("Failed to search blocks: %v", err)
	}

	if len(blocks) != 2 {
		t.Errorf("Expected 2 stored blocks, got %d", len(blocks))
	}

	// Verify streamed blocks match the test data
	for i, streamedBlock := range streamedBlocks {
		expectedText := testBlocks[i].Text()
		if streamedBlock.Text() != expectedText {
			t.Errorf("Streamed block %d text mismatch. Expected %s, got %s", i, expectedText, streamedBlock.Text())
		}
	}
}

func TestFetchOnceAPIVariations(t *testing.T) {
	storageManager, err := storage.NewManager(t.TempDir())
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	config := Config{
		OptimizeInterval: 0,
	}
	wh := NewWarehouse(config, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Logf("Warning: failed to close warehouse: %v", err)
		}
	}()

	now := time.Now()
	testBlocks := []core.Block{
		&mockBlock{
			id:        "api1",
			text:      "API test block 1",
			createdAt: now,
			source:    "test-datasource",
			metadata:  map[string]interface{}{"test": "api"},
		},
		&mockBlock{
			id:        "api2",
			text:      "API test block 2",
			createdAt: now.Add(time.Minute),
			source:    "test-datasource",
			metadata:  map[string]interface{}{"test": "api"},
		},
	}

	mockDS := &mockDatasource{
		name:   "test-datasource",
		blocks: testBlocks,
	}

	err = wh.AddDatasource("test-datasource", mockDS)
	if err != nil {
		t.Fatalf("Failed to add datasource: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Test 1: Silent fetch (no options)
	t.Run("SilentFetch", func(t *testing.T) {
		err := wh.FetchOnce(ctx)
		if err != nil {
			t.Fatalf("Failed silent fetch: %v", err)
		}
	})

	// Test 2: Streaming fetch with callback
	t.Run("StreamingFetch", func(t *testing.T) {
		var capturedBlocks []string
		err := wh.FetchOnce(ctx, WithStreaming(func(block core.Block) {
			capturedBlocks = append(capturedBlocks, block.Text())
		}))
		if err != nil {
			t.Fatalf("Failed streaming fetch: %v", err)
		}

		if len(capturedBlocks) != 2 {
			t.Errorf("Expected 2 captured blocks, got %d", len(capturedBlocks))
		}
	})

	// Verify all blocks were stored
	blocks, err := wh.SearchBlocks("test-datasource", "", 10)
	if err != nil {
		t.Fatalf("Failed to search blocks: %v", err)
	}

	// Should have blocks from both runs (but deduplicated by ID)
	if len(blocks) != 2 {
		t.Errorf("Expected 2 total blocks, got %d", len(blocks))
	}
}

// TestIsDatasourceConfiguredAndDropUnknown verifies that:
//  1. isDatasourceConfigured returns true for added datasources and false otherwise
//  2. Blocks from unknown/disabled datasources are dropped (no DB created)
func TestIsDatasourceConfiguredAndDropUnknown(t *testing.T) {
	tempDir := t.TempDir()
	storageManager, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	wh := NewWarehouse(Config{OptimizeInterval: 0}, storageManager)
	defer func() {
		if err := wh.Close(); err != nil {
			t.Logf("Warning: failed to close warehouse: %v", err)
		}
	}()

	// Add a known datasource
	dsName := "known-ds"
	mockDS := &mockDatasource{name: dsName}
	if err := wh.AddDatasource(dsName, mockDS); err != nil {
		t.Fatalf("Failed to add datasource: %v", err)
	}

	// Positive check
	if !wh.isDatasourceConfigured(dsName) {
		t.Fatalf("Expected datasource %s to be configured", dsName)
	}

	// Negative check
	unknown := "unknown-ds"
	if wh.isDatasourceConfigured(unknown) {
		t.Fatalf("Did not expect datasource %s to be configured", unknown)
	}

	// Attempt to store a block from unknown datasource (should be dropped silently)
	unknownBlock := &mockBlock{
		id:        "u1",
		text:      "Should be dropped",
		createdAt: time.Now(),
		source:    unknown,
		metadata:  map[string]interface{}{},
	}
	if err := wh.storeBlock(unknownBlock); err != nil {
		t.Fatalf("storeBlock for unknown datasource returned unexpected error: %v", err)
	}

	// Ensure DB for unknown datasource was NOT created
	if _, statErr := os.Stat(filepath.Join(tempDir, unknown+".db")); statErr == nil {
		t.Fatalf("Database file for unknown datasource %s was created unexpectedly", unknown)
	}

	// Store a block for the known datasource to ensure normal path still works
	knownBlock := &mockBlock{
		id:        "k1",
		text:      "Should persist",
		createdAt: time.Now(),
		source:    dsName,
		metadata:  map[string]interface{}{},
	}
	if err := wh.storeBlock(knownBlock); err != nil {
		t.Fatalf("storeBlock for known datasource failed: %v", err)
	}

	// Verify the known block was stored
	results, err := wh.SearchBlocks(dsName, "Should persist", 10)
	if err != nil {
		t.Fatalf("SearchBlocks failed: %v", err)
	}
	found := false
	for _, b := range results {
		if b.ID() == "k1" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("Expected to find stored block k1 for datasource %s", dsName)
	}
}
