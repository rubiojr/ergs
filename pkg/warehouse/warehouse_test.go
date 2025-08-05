package warehouse

import (
	"context"
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
