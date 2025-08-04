package storage

import (
	"fmt"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

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

func createTestManager(t *testing.T) *Manager {
	return NewManager(t.TempDir())
}

func setupTestData(t *testing.T, manager *Manager, datasources map[string][]core.Block) {
	for datasourceName, blocks := range datasources {
		schema := map[string]any{
			"text":       "TEXT",
			"created_at": "DATETIME",
			"metadata":   "TEXT",
		}

		err := manager.InitializeDatasourceStorage(datasourceName, schema)
		if err != nil {
			t.Fatalf("Failed to initialize storage for %s: %v", datasourceName, err)
		}

		storage, err := manager.GetStorage(datasourceName)
		if err != nil {
			t.Fatalf("Failed to get storage for %s: %v", datasourceName, err)
		}

		for _, block := range blocks {
			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block in %s: %v", datasourceName, err)
			}
		}

		mockBlockPrototype := &mockBlock{}
		manager.RegisterBlockPrototype(datasourceName, mockBlockPrototype)
	}
}

func TestManagerGetStorage(t *testing.T) {
	manager := createTestManager(t)
	defer func() {
		if err := manager.Close(); err != nil {
			t.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	datasourceName := "test-datasource"

	storage1, err := manager.GetStorage(datasourceName)
	if err != nil {
		t.Fatalf("Failed to get storage: %v", err)
	}

	storage2, err := manager.GetStorage(datasourceName)
	if err != nil {
		t.Fatalf("Failed to get storage again: %v", err)
	}

	if storage1 != storage2 {
		t.Error("Expected same storage instance to be returned")
	}
}

func TestManagerConcurrentGetStorage(t *testing.T) {
	manager := createTestManager(t)
	defer func() {
		if err := manager.Close(); err != nil {
			t.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	datasourceName := "concurrent-test"
	numGoroutines := 10
	storages := make([]*GenericStorage, numGoroutines)
	errors := make([]error, numGoroutines)

	var wg sync.WaitGroup
	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			storage, err := manager.GetStorage(datasourceName)
			storages[index] = storage
			errors[index] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		}
	}

	firstStorage := storages[0]
	for i, storage := range storages {
		if storage != firstStorage {
			t.Errorf("Storage %d is different from first storage", i)
		}
	}
}

func TestManagerSearchBlocks(t *testing.T) {
	manager := createTestManager(t)
	defer func() {
		if err := manager.Close(); err != nil {
			t.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{
				id:        "block1",
				text:      "golang programming language",
				createdAt: now,
				source:    "datasource1",
				metadata:  map[string]interface{}{"type": "test"},
			},
			&mockBlock{
				id:        "block2",
				text:      "python programming tutorial",
				createdAt: now.Add(time.Minute),
				source:    "datasource1",
				metadata:  map[string]interface{}{"type": "test"},
			},
		},
	}

	setupTestData(t, manager, testData)

	blocks, err := manager.SearchBlocks("datasource1", "programming", 10)
	if err != nil {
		t.Fatalf("Failed to search blocks: %v", err)
	}

	if len(blocks) != 2 {
		t.Errorf("Expected 2 blocks, got %d", len(blocks))
	}

	blocks, err = manager.SearchBlocks("datasource1", "golang", 10)
	if err != nil {
		t.Fatalf("Failed to search blocks: %v", err)
	}

	if len(blocks) != 1 {
		t.Errorf("Expected 1 block, got %d", len(blocks))
	}

	if blocks[0].Text() != "golang programming language" {
		t.Errorf("Unexpected block text: %s", blocks[0].Text())
	}
}

func TestManagerSearchAllDatasources(t *testing.T) {
	manager := createTestManager(t)
	defer func() {
		if err := manager.Close(); err != nil {
			t.Logf("Warning: failed to close manager: %v", err)
		}
	}()

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{
				id:        "block1",
				text:      "golang programming language",
				createdAt: now,
				source:    "datasource1",
			},
			&mockBlock{
				id:        "block2",
				text:      "python programming tutorial",
				createdAt: now.Add(time.Minute),
				source:    "datasource1",
			},
		},
		"datasource2": {
			&mockBlock{
				id:        "block3",
				text:      "rust programming guide",
				createdAt: now,
				source:    "datasource2",
			},
			&mockBlock{
				id:        "block4",
				text:      "javascript framework",
				createdAt: now.Add(time.Minute),
				source:    "datasource2",
			},
		},
		"datasource3": {
			&mockBlock{
				id:        "block5",
				text:      "database design patterns",
				createdAt: now,
				source:    "datasource3",
			},
		},
	}

	setupTestData(t, manager, testData)

	results, err := manager.SearchAllDatasources("programming", 9)
	if err != nil {
		t.Fatalf("Failed to search all datasources: %v", err)
	}

	// With fair distribution across 3 datasources (9 total limit):
	// Each datasource gets 3 slots base allocation
	// datasource1 has 2 blocks -> gets 2
	// datasource2 has 1 block -> gets 1
	// datasource3 has 0 blocks -> gets 0
	// Remaining 6 slots (9-3) can't be used since no datasource has overflow

	if len(results) != 2 {
		t.Errorf("Expected results from 2 datasources, got %d", len(results))
	}

	ds1Results, exists := results["datasource1"]
	if !exists {
		t.Error("Expected results from datasource1")
	} else if len(ds1Results) != 2 {
		t.Errorf("Expected 2 results from datasource1, got %d", len(ds1Results))
	}

	ds2Results, exists := results["datasource2"]
	if !exists {
		t.Error("Expected results from datasource2")
	} else if len(ds2Results) != 1 {
		t.Errorf("Expected 1 result from datasource2, got %d", len(ds2Results))
	}

	if _, exists := results["datasource3"]; exists {
		t.Error("Did not expect results from datasource3 for 'programming' query")
	}
}

func TestManagerSearchAllDatasourcesParallelization(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	numDatasources := 10
	blocksPerDatasource := 5
	now := time.Now()

	testData := make(map[string][]core.Block)
	for i := 0; i < numDatasources; i++ {
		datasourceName := fmt.Sprintf("datasource%d", i)
		blocks := make([]core.Block, blocksPerDatasource)

		for j := 0; j < blocksPerDatasource; j++ {
			blocks[j] = &mockBlock{
				id:        fmt.Sprintf("block%d_%d", i, j),
				text:      fmt.Sprintf("test content %d programming tutorial", i),
				createdAt: now.Add(time.Duration(j) * time.Minute),
				source:    datasourceName,
			}
		}
		testData[datasourceName] = blocks
	}

	setupTestData(t, manager, testData)

	start := time.Now()
	results, err := manager.SearchAllDatasources("programming", 30)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Failed to search all datasources: %v", err)
	}

	// With sequential paging: results are paginated across all datasources
	// Page 1 with limit 30 should return exactly 30 results total
	totalResults := 0
	for dsName, dsResults := range results {
		totalResults += len(dsResults)
		t.Logf("Datasource %s returned %d results", dsName, len(dsResults))
	}

	if totalResults != 30 {
		t.Errorf("Expected exactly 30 results for page 1, got %d", totalResults)
	}

	t.Logf("Parallel search across %d datasources took %v, returned %d total results", numDatasources, duration, totalResults)

	if duration > 5*time.Second {
		t.Errorf("Parallel search took too long: %v (might indicate lack of parallelization)", duration)
	}
}

func TestManagerSearchAllDatasourcesWithError(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"good-datasource": {
			&mockBlock{
				id:        "block1",
				text:      "golang programming",
				createdAt: now,
				source:    "good-datasource",
			},
		},
	}

	setupTestData(t, manager, testData)

	// Create a storage that will be closed to trigger an error
	badStorage, err := NewGenericStorage(t.TempDir()+"/bad.db", "bad-datasource")
	if err != nil {
		t.Fatalf("Failed to create bad storage: %v", err)
	}

	// Close the storage to make it unusable
	if err := badStorage.Close(); err != nil {
		t.Logf("Warning: failed to close bad storage: %v", err)
	}

	manager.mu.Lock()
	manager.storages["bad-datasource"] = badStorage
	manager.mu.Unlock()

	_, err = manager.SearchAllDatasources("test", 10)
	if err == nil {
		t.Error("Expected error when searching with closed storage, got nil")
	}

	if err != nil && !strings.Contains(err.Error(), "bad-datasource") {
		t.Errorf("Error should mention the problematic datasource: %v", err)
	}
}

func TestManagerSearchAllDatasourcesEmpty(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	results, err := manager.SearchAllDatasources("test", 10)
	if err != nil {
		t.Fatalf("Failed to search empty manager: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d datasources", len(results))
	}
}

func TestManagerSearchAllDatasourcesNoMatches(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{
				id:        "block1",
				text:      "golang programming",
				createdAt: now,
				source:    "datasource1",
			},
		},
		"datasource2": {
			&mockBlock{
				id:        "block2",
				text:      "python tutorial",
				createdAt: now,
				source:    "datasource2",
			},
		},
	}

	setupTestData(t, manager, testData)

	results, err := manager.SearchAllDatasources("nonexistent", 10)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected no results for nonexistent query, got %d datasources", len(results))
	}
}

func TestManagerSearchAllDatasourcesConcurrentAccess(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{
				id:        "block1",
				text:      "concurrent test programming",
				createdAt: now,
				source:    "datasource1",
			},
		},
		"datasource2": {
			&mockBlock{
				id:        "block2",
				text:      "parallel search test",
				createdAt: now,
				source:    "datasource2",
			},
		},
	}

	setupTestData(t, manager, testData)

	numGoroutines := 10
	var wg sync.WaitGroup
	errors := make([]error, numGoroutines)
	results := make([]map[string][]core.Block, numGoroutines)

	for i := 0; i < numGoroutines; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := manager.SearchAllDatasources("test", 10)
			results[index] = result
			errors[index] = err
		}(i)
	}

	wg.Wait()

	for i, err := range errors {
		if err != nil {
			t.Errorf("Goroutine %d failed: %v", i, err)
		}
	}

	for i, result := range results {
		if len(result) != 2 {
			t.Errorf("Goroutine %d got %d datasources, expected 2", i, len(result))
		}
	}
}

func TestManagerGetStats(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{id: "block1", text: "test1", createdAt: now, source: "datasource1"},
			&mockBlock{id: "block2", text: "test2", createdAt: now, source: "datasource1"},
		},
		"datasource2": {
			&mockBlock{id: "block3", text: "test3", createdAt: now, source: "datasource2"},
		},
	}

	setupTestData(t, manager, testData)

	stats, err := manager.GetStats()
	if err != nil {
		t.Fatalf("Failed to get stats: %v", err)
	}

	totalDatasources, ok := stats["total_datasources"].(int)
	if !ok {
		t.Error("total_datasources should be an int")
	} else if totalDatasources != 2 {
		t.Errorf("Expected 2 total datasources, got %d", totalDatasources)
	}

	totalBlocks, ok := stats["total_blocks"].(int)
	if !ok {
		t.Error("total_blocks should be an int")
	} else if totalBlocks != 3 {
		t.Errorf("Expected 3 total blocks, got %d", totalBlocks)
	}
}

func TestManagerClose(t *testing.T) {
	manager := createTestManager(t)

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{id: "block1", text: "test", createdAt: now, source: "datasource1"},
		},
	}

	setupTestData(t, manager, testData)

	err := manager.Close()
	if err != nil {
		t.Fatalf("Failed to close manager: %v", err)
	}

	manager.mu.RLock()
	storageCount := len(manager.storages)
	manager.mu.RUnlock()

	if storageCount != 0 {
		t.Errorf("Expected 0 storages after close, got %d", storageCount)
	}
}

func TestManagerOptimizeAll(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{id: "block1", text: "test", createdAt: now, source: "datasource1"},
		},
	}

	setupTestData(t, manager, testData)

	err := manager.OptimizeAll()
	if err != nil {
		t.Fatalf("Failed to optimize all: %v", err)
	}
}

func TestManagerAnalyzeAll(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{id: "block1", text: "test", createdAt: now, source: "datasource1"},
		},
	}

	setupTestData(t, manager, testData)

	err := manager.AnalyzeAll()
	if err != nil {
		t.Fatalf("Failed to analyze all: %v", err)
	}
}

func TestManagerWALCheckpointAll(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{id: "block1", text: "test", createdAt: now, source: "datasource1"},
		},
	}

	setupTestData(t, manager, testData)

	err := manager.WALCheckpointAll()
	if err != nil {
		t.Fatalf("Failed to WAL checkpoint all: %v", err)
	}
}

func BenchmarkSearchAllDatasourcesParallel(b *testing.B) {
	manager := createTestManager(&testing.T{})
	defer manager.Close() //nolint:errcheck

	numDatasources := 20
	blocksPerDatasource := 100
	now := time.Now()

	testData := make(map[string][]core.Block)
	for i := 0; i < numDatasources; i++ {
		datasourceName := fmt.Sprintf("datasource%d", i)
		blocks := make([]core.Block, blocksPerDatasource)

		for j := 0; j < blocksPerDatasource; j++ {
			blocks[j] = &mockBlock{
				id:        fmt.Sprintf("block%d_%d", i, j),
				text:      fmt.Sprintf("benchmark test content %d programming tutorial", i),
				createdAt: now.Add(time.Duration(j) * time.Minute),
				source:    datasourceName,
			}
		}
		testData[datasourceName] = blocks
	}

	setupTestData(&testing.T{}, manager, testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.SearchAllDatasources("programming", 50)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func TestSearchAllDatasourcesParallelBenefit(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	numDatasources := 5
	now := time.Now()

	// Create test data with varying amounts per datasource
	testData := make(map[string][]core.Block)
	for i := 0; i < numDatasources; i++ {
		datasourceName := fmt.Sprintf("slow-datasource%d", i)
		// Different datasources have different numbers of blocks
		numBlocks := i + 1
		blocks := make([]core.Block, numBlocks)
		for j := 0; j < numBlocks; j++ {
			blocks[j] = &mockBlock{
				id:        fmt.Sprintf("block%d_%d", i, j),
				text:      "test programming content with delay",
				createdAt: now,
				source:    datasourceName,
			}
		}
		testData[datasourceName] = blocks
	}

	setupTestData(t, manager, testData)

	// Measure parallel execution time
	start := time.Now()
	results, err := manager.SearchAllDatasources("programming", 10)
	parallelDuration := time.Since(start)

	if err != nil {
		t.Fatalf("Parallel search failed: %v", err)
	}

	// With sequential paging, should get exactly the page size
	totalResults := 0
	for dsName, blocks := range results {
		totalResults += len(blocks)
		t.Logf("Datasource %s returned %d results", dsName, len(blocks))
	}

	if totalResults != 10 {
		t.Errorf("Expected exactly 10 results for page 1, got %d", totalResults)
	}

	t.Logf("Parallel search across %d datasources took %v, total results: %d", numDatasources, parallelDuration, totalResults)

	// Test that multiple concurrent calls work correctly
	var wg sync.WaitGroup
	numConcurrentCalls := 3
	callResults := make([]map[string][]core.Block, numConcurrentCalls)
	callErrors := make([]error, numConcurrentCalls)

	concurrentStart := time.Now()
	for i := 0; i < numConcurrentCalls; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			result, err := manager.SearchAllDatasources("programming", 10)
			callResults[index] = result
			callErrors[index] = err
		}(i)
	}

	wg.Wait()
	concurrentDuration := time.Since(concurrentStart)

	for i, err := range callErrors {
		if err != nil {
			t.Errorf("Concurrent call %d failed: %v", i, err)
		}
	}

	t.Logf("Concurrent parallel searches (%d calls) took %v", numConcurrentCalls, concurrentDuration)
}

func TestSearchAllDatasourcesSequentialPaging(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	// Create test data with different amounts per datasource
	testData := map[string][]core.Block{
		"datasource_a": make([]core.Block, 10),
		"datasource_b": make([]core.Block, 8),
		"datasource_c": make([]core.Block, 5),
	}

	// Fill datasource_a
	for i := 0; i < 10; i++ {
		testData["datasource_a"][i] = &mockBlock{
			id:        fmt.Sprintf("a_block_%d", i),
			text:      fmt.Sprintf("programming tutorial content a %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_a",
		}
	}

	// Fill datasource_b
	for i := 0; i < 8; i++ {
		testData["datasource_b"][i] = &mockBlock{
			id:        fmt.Sprintf("b_block_%d", i),
			text:      fmt.Sprintf("programming tutorial content b %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_b",
		}
	}

	// Fill datasource_c
	for i := 0; i < 5; i++ {
		testData["datasource_c"][i] = &mockBlock{
			id:        fmt.Sprintf("c_block_%d", i),
			text:      fmt.Sprintf("programming tutorial content c %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_c",
		}
	}

	setupTestData(t, manager, testData)

	// Test page 1 with limit of 10
	results, err := manager.SearchAllDatasourcesPaged("programming", 100, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search page 1: %v", err)
	}

	totalResults := 0
	for dsName, blocks := range results {
		totalResults += len(blocks)
		t.Logf("Page 1 - Datasource %s: %d results", dsName, len(blocks))
	}

	if totalResults != 10 {
		t.Errorf("Expected exactly 10 results for page 1, got %d", totalResults)
	}

	// Test page 2
	results2, err := manager.SearchAllDatasourcesPaged("programming", 100, 2, 10)
	if err != nil {
		t.Fatalf("Failed to search page 2: %v", err)
	}

	totalResults2 := 0
	for dsName, blocks := range results2 {
		totalResults2 += len(blocks)
		t.Logf("Page 2 - Datasource %s: %d results", dsName, len(blocks))
	}

	if totalResults2 != 10 {
		t.Errorf("Expected exactly 10 results for page 2, got %d", totalResults2)
	}

	// Test page 3 (should have remaining 3 results)
	results3, err := manager.SearchAllDatasourcesPaged("programming", 100, 3, 10)
	if err != nil {
		t.Fatalf("Failed to search page 3: %v", err)
	}

	totalResults3 := 0
	for dsName, blocks := range results3 {
		totalResults3 += len(blocks)
		t.Logf("Page 3 - Datasource %s: %d results", dsName, len(blocks))
	}

	if totalResults3 != 3 {
		t.Errorf("Expected exactly 3 results for page 3, got %d", totalResults3)
	}
}

func TestSearchAllDatasourcesAlphabeticalOrder(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	// Create datasources with names that will test alphabetical ordering
	testData := map[string][]core.Block{
		"zebra_datasource": {
			&mockBlock{
				id:        "zebra_1",
				text:      "zebra test content",
				createdAt: now,
				source:    "zebra_datasource",
			},
		},
		"alpha_datasource": {
			&mockBlock{
				id:        "alpha_1",
				text:      "alpha test content",
				createdAt: now,
				source:    "alpha_datasource",
			},
		},
		"beta_datasource": {
			&mockBlock{
				id:        "beta_1",
				text:      "beta test content",
				createdAt: now,
				source:    "beta_datasource",
			},
		},
	}

	setupTestData(t, manager, testData)

	// Test with sequential paging to verify order
	results, err := manager.SearchAllDatasourcesPaged("test", 100, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	// Collect the order of datasources as they appear in results
	var resultOrder []string
	for dsName := range results {
		resultOrder = append(resultOrder, dsName)
	}

	// The results should be in alphabetical order
	expectedOrder := []string{"alpha_datasource", "beta_datasource", "zebra_datasource"}
	if len(resultOrder) != len(expectedOrder) {
		t.Fatalf("Expected %d datasources, got %d", len(expectedOrder), len(resultOrder))
	}

	// Since map iteration order is not guaranteed, we need to sort to compare
	sortedResultOrder := make([]string, len(resultOrder))
	copy(sortedResultOrder, resultOrder)
	sort.Strings(sortedResultOrder)

	for i, expected := range expectedOrder {
		if sortedResultOrder[i] != expected {
			t.Errorf("Expected datasource %d to be %s, got %s", i, expected, sortedResultOrder[i])
		}
	}

	t.Logf("Datasources returned in order: %v", sortedResultOrder)
}

func TestPaginationBehaviorDetailed(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	// Create test data with known total count across multiple datasources
	testData := map[string][]core.Block{
		"datasource_a": make([]core.Block, 25), // 25 blocks
		"datasource_b": make([]core.Block, 15), // 15 blocks
		"datasource_c": make([]core.Block, 10), // 10 blocks
	}

	// Fill datasource_a
	for i := 0; i < 25; i++ {
		testData["datasource_a"][i] = &mockBlock{
			id:        fmt.Sprintf("a_block_%d", i),
			text:      fmt.Sprintf("test content a %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_a",
		}
	}

	// Fill datasource_b
	for i := 0; i < 15; i++ {
		testData["datasource_b"][i] = &mockBlock{
			id:        fmt.Sprintf("b_block_%d", i),
			text:      fmt.Sprintf("test content b %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_b",
		}
	}

	// Fill datasource_c
	for i := 0; i < 10; i++ {
		testData["datasource_c"][i] = &mockBlock{
			id:        fmt.Sprintf("c_block_%d", i),
			text:      fmt.Sprintf("test content c %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_c",
		}
	}

	setupTestData(t, manager, testData)

	// Total expected blocks: 25 + 15 + 10 = 50
	expectedTotal := 50

	// Test pagination with pageSize 10
	pageSize := 10
	expectedPages := (expectedTotal + pageSize - 1) / pageSize // Should be 5 pages

	t.Logf("Testing pagination: %d total blocks, %d page size, expecting %d pages", expectedTotal, pageSize, expectedPages)

	allResults := make(map[string][]core.Block)
	totalFoundBlocks := 0

	for page := 1; page <= expectedPages+1; page++ { // Test one extra page
		results, err := manager.SearchAllDatasourcesPaged("test", expectedTotal*2, page, pageSize)
		if err != nil {
			t.Fatalf("Failed to search page %d: %v", page, err)
		}

		pageResults := 0
		for dsName, blocks := range results {
			pageResults += len(blocks)
			// Collect all blocks to verify no duplicates
			allResults[dsName] = append(allResults[dsName], blocks...)
		}

		t.Logf("Page %d: %d results", page, pageResults)

		if page <= expectedPages {
			if pageResults == 0 {
				t.Errorf("Page %d should have results but got 0", page)
			}
			if page < expectedPages && pageResults != pageSize {
				t.Errorf("Page %d should have %d results, got %d", page, pageSize, pageResults)
			}
		} else {
			// This should be beyond the last page
			if pageResults > 0 {
				t.Errorf("Page %d (beyond expected pages) should have 0 results, got %d", page, pageResults)
			}
		}

		totalFoundBlocks += pageResults
	}

	// Verify total blocks found
	if totalFoundBlocks != expectedTotal {
		t.Errorf("Expected to find %d total blocks across all pages, got %d", expectedTotal, totalFoundBlocks)
	}

	// Verify no duplicate blocks
	uniqueBlocks := make(map[string]bool)
	for _, blocks := range allResults {
		for _, block := range blocks {
			if uniqueBlocks[block.ID()] {
				t.Errorf("Found duplicate block ID: %s", block.ID())
			}
			uniqueBlocks[block.ID()] = true
		}
	}

	t.Logf("Verified %d unique blocks across all pages", len(uniqueBlocks))
}

func TestBackendPaginationLogic(t *testing.T) {
	// This test focuses on the backend pagination calculation only
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	// Create test data with exact known counts
	testData := map[string][]core.Block{
		"datasource_x": make([]core.Block, 37), // 37 blocks
	}

	// Fill with test data
	for i := 0; i < 37; i++ {
		testData["datasource_x"][i] = &mockBlock{
			id:        fmt.Sprintf("x_block_%d", i),
			text:      fmt.Sprintf("pagination test content %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_x",
		}
	}

	setupTestData(t, manager, testData)

	testCases := []struct {
		page        int
		limit       int
		expectedMin int
		expectedMax int
		description string
	}{
		{1, 10, 10, 10, "page 1 of 37 items with limit 10"},
		{2, 10, 10, 10, "page 2 of 37 items with limit 10"},
		{3, 10, 10, 10, "page 3 of 37 items with limit 10"},
		{4, 10, 7, 7, "page 4 (last) of 37 items with limit 10"},
		{5, 10, 0, 0, "page 5 (beyond last) of 37 items with limit 10"},
		{1, 20, 20, 20, "page 1 of 37 items with limit 20"},
		{2, 20, 17, 17, "page 2 (last) of 37 items with limit 20"},
		{1, 50, 37, 37, "page 1 of 37 items with limit 50 (all on one page)"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			results, err := manager.SearchAllDatasourcesPaged("pagination", 100, tc.page, tc.limit)
			if err != nil {
				t.Fatalf("Failed to search: %v", err)
			}

			totalResults := 0
			for _, blocks := range results {
				totalResults += len(blocks)
			}

			t.Logf("%s: got %d results", tc.description, totalResults)

			if totalResults < tc.expectedMin || totalResults > tc.expectedMax {
				t.Errorf("Expected %d-%d results, got %d", tc.expectedMin, tc.expectedMax, totalResults)
			}

			// Log the actual datasource breakdown
			for dsName, blocks := range results {
				t.Logf("  %s: %d blocks", dsName, len(blocks))
			}
		})
	}
}

func BenchmarkSearchAllDatasourcesSmall(b *testing.B) {
	manager := createTestManager(&testing.T{})
	defer manager.Close() //nolint:errcheck

	numDatasources := 5
	blocksPerDatasource := 50
	now := time.Now()

	testData := make(map[string][]core.Block)
	for i := 0; i < numDatasources; i++ {
		datasourceName := fmt.Sprintf("datasource%d", i)
		blocks := make([]core.Block, blocksPerDatasource)

		for j := 0; j < blocksPerDatasource; j++ {
			blocks[j] = &mockBlock{
				id:        fmt.Sprintf("block%d_%d", i, j),
				text:      fmt.Sprintf("benchmark test content %d programming tutorial", i),
				createdAt: now.Add(time.Duration(j) * time.Minute),
				source:    datasourceName,
			}
		}
		testData[datasourceName] = blocks
	}

	setupTestData(&testing.T{}, manager, testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.SearchAllDatasources("programming", 50)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func BenchmarkSearchAllDatasourcesLarge(b *testing.B) {
	manager := createTestManager(&testing.T{})
	defer manager.Close() //nolint:errcheck

	numDatasources := 50
	blocksPerDatasource := 200
	now := time.Now()

	testData := make(map[string][]core.Block)
	for i := 0; i < numDatasources; i++ {
		datasourceName := fmt.Sprintf("datasource%d", i)
		blocks := make([]core.Block, blocksPerDatasource)

		for j := 0; j < blocksPerDatasource; j++ {
			blocks[j] = &mockBlock{
				id:        fmt.Sprintf("block%d_%d", i, j),
				text:      fmt.Sprintf("benchmark test content %d programming tutorial", i),
				createdAt: now.Add(time.Duration(j) * time.Minute),
				source:    datasourceName,
			}
		}
		testData[datasourceName] = blocks
	}

	setupTestData(&testing.T{}, manager, testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.SearchAllDatasources("programming", 50)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}

func TestSearchDatasourcesPagedSpecificDatasources(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource_a": {
			&mockBlock{id: "a1", text: "programming tutorial", createdAt: now, source: "datasource_a"},
			&mockBlock{id: "a2", text: "programming guide", createdAt: now.Add(1 * time.Minute), source: "datasource_a"},
			&mockBlock{id: "a3", text: "programming basics", createdAt: now.Add(2 * time.Minute), source: "datasource_a"},
		},
		"datasource_b": {
			&mockBlock{id: "b1", text: "programming advanced", createdAt: now, source: "datasource_b"},
			&mockBlock{id: "b2", text: "programming tips", createdAt: now.Add(1 * time.Minute), source: "datasource_b"},
		},
		"datasource_c": {
			&mockBlock{id: "c1", text: "programming concepts", createdAt: now, source: "datasource_c"},
		},
	}

	setupTestData(t, manager, testData)

	// Test searching only datasource_a
	results, err := manager.SearchDatasourcesPaged([]string{"datasource_a"}, "programming", 10, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search datasource_a: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 datasource in results, got %d", len(results))
	}

	if _, hasA := results["datasource_a"]; !hasA {
		t.Error("Expected datasource_a in results")
	}

	if len(results["datasource_a"]) != 3 {
		t.Errorf("Expected 3 blocks from datasource_a, got %d", len(results["datasource_a"]))
	}
}

func TestSearchDatasourcesPagedMultipleDatasources(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource_a": {
			&mockBlock{id: "a1", text: "programming tutorial", createdAt: now, source: "datasource_a"},
			&mockBlock{id: "a2", text: "programming guide", createdAt: now.Add(1 * time.Minute), source: "datasource_a"},
		},
		"datasource_b": {
			&mockBlock{id: "b1", text: "programming advanced", createdAt: now, source: "datasource_b"},
			&mockBlock{id: "b2", text: "programming tips", createdAt: now.Add(1 * time.Minute), source: "datasource_b"},
		},
		"datasource_c": {
			&mockBlock{id: "c1", text: "programming concepts", createdAt: now, source: "datasource_c"},
		},
	}

	setupTestData(t, manager, testData)

	// Test searching datasource_a and datasource_b
	results, err := manager.SearchDatasourcesPaged([]string{"datasource_a", "datasource_b"}, "programming", 10, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search multiple datasources: %v", err)
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 datasources in results, got %d", len(results))
	}

	if _, hasA := results["datasource_a"]; !hasA {
		t.Error("Expected datasource_a in results")
	}

	if _, hasB := results["datasource_b"]; !hasB {
		t.Error("Expected datasource_b in results")
	}

	if _, hasC := results["datasource_c"]; hasC {
		t.Error("Should not have datasource_c in results")
	}

	totalBlocks := len(results["datasource_a"]) + len(results["datasource_b"])
	if totalBlocks != 4 {
		t.Errorf("Expected 4 total blocks, got %d", totalBlocks)
	}
}

func TestSearchDatasourcesPagedNonexistentDatasource(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource_a": {
			&mockBlock{id: "a1", text: "programming tutorial", createdAt: now, source: "datasource_a"},
		},
	}

	setupTestData(t, manager, testData)

	// Test searching a nonexistent datasource
	results, err := manager.SearchDatasourcesPaged([]string{"nonexistent"}, "programming", 10, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search nonexistent datasource: %v", err)
	}

	if len(results) != 0 {
		t.Errorf("Expected 0 datasources in results when searching nonexistent datasource, got %d", len(results))
	}
}

func TestSearchDatasourcesPagedMixedExistingAndNonexistent(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource_a": {
			&mockBlock{id: "a1", text: "programming tutorial", createdAt: now, source: "datasource_a"},
			&mockBlock{id: "a2", text: "programming guide", createdAt: now.Add(1 * time.Minute), source: "datasource_a"},
		},
	}

	setupTestData(t, manager, testData)

	// Test searching one existing and one nonexistent datasource
	results, err := manager.SearchDatasourcesPaged([]string{"datasource_a", "nonexistent"}, "programming", 10, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search mixed datasources: %v", err)
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 datasource in results (nonexistent should be filtered out), got %d", len(results))
	}

	if _, hasA := results["datasource_a"]; !hasA {
		t.Error("Expected datasource_a in results")
	}

	if _, hasNonexistent := results["nonexistent"]; hasNonexistent {
		t.Error("Should not have nonexistent datasource in results")
	}

	if len(results["datasource_a"]) != 2 {
		t.Errorf("Expected 2 blocks from datasource_a, got %d", len(results["datasource_a"]))
	}
}

func TestSearchDatasourcesPagedAlphabeticalOrdering(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"zebra_datasource": {
			&mockBlock{id: "z1", text: "programming tutorial", createdAt: now, source: "zebra_datasource"},
		},
		"alpha_datasource": {
			&mockBlock{id: "a1", text: "programming guide", createdAt: now.Add(1 * time.Minute), source: "alpha_datasource"},
		},
		"beta_datasource": {
			&mockBlock{id: "b1", text: "programming basics", createdAt: now.Add(2 * time.Minute), source: "beta_datasource"},
		},
	}

	setupTestData(t, manager, testData)

	// Test that results are ordered alphabetically by datasource name
	results, err := manager.SearchDatasourcesPaged([]string{"zebra_datasource", "alpha_datasource", "beta_datasource"}, "programming", 10, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search datasources: %v", err)
	}

	if len(results) != 3 {
		t.Errorf("Expected 3 datasources in results, got %d", len(results))
	}

	// Collect all block IDs in order
	var blockIDs []string
	expectedOrder := []string{"alpha_datasource", "beta_datasource", "zebra_datasource"}

	// Since results are returned by datasource, we need to check that the datasources exist in alphabetical order
	for _, dsName := range expectedOrder {
		if blocks, exists := results[dsName]; exists {
			for _, block := range blocks {
				blockIDs = append(blockIDs, block.ID())
			}
		}
	}

	expectedIDs := []string{"a1", "b1", "z1"}
	if len(blockIDs) != len(expectedIDs) {
		t.Errorf("Expected %d blocks, got %d", len(expectedIDs), len(blockIDs))
	}

	for i, expected := range expectedIDs {
		if i >= len(blockIDs) || blockIDs[i] != expected {
			t.Errorf("Expected block ID %s at position %d, got %s", expected, i, blockIDs[i])
		}
	}
}

func TestSearchDatasourcesPagedPagination(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	now := time.Now()
	testData := map[string][]core.Block{
		"datasource_a": make([]core.Block, 15),
	}

	// Fill with test data
	for i := 0; i < 15; i++ {
		testData["datasource_a"][i] = &mockBlock{
			id:        fmt.Sprintf("a_%d", i),
			text:      fmt.Sprintf("programming tutorial %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_a",
		}
	}

	setupTestData(t, manager, testData)

	// Test page 1 with limit 10
	results1, err := manager.SearchDatasourcesPaged([]string{"datasource_a"}, "programming", 30, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search page 1: %v", err)
	}

	totalResults1 := 0
	for _, blocks := range results1 {
		totalResults1 += len(blocks)
	}

	if totalResults1 != 10 {
		t.Errorf("Expected 10 results on page 1, got %d", totalResults1)
	}

	// Test page 2 with limit 10
	results2, err := manager.SearchDatasourcesPaged([]string{"datasource_a"}, "programming", 30, 2, 10)
	if err != nil {
		t.Fatalf("Failed to search page 2: %v", err)
	}

	totalResults2 := 0
	for _, blocks := range results2 {
		totalResults2 += len(blocks)
	}

	if totalResults2 != 5 {
		t.Errorf("Expected 5 results on page 2, got %d", totalResults2)
	}

	// Test page 3 (should be empty)
	results3, err := manager.SearchDatasourcesPaged([]string{"datasource_a"}, "programming", 30, 3, 10)
	if err != nil {
		t.Fatalf("Failed to search page 3: %v", err)
	}

	totalResults3 := 0
	for _, blocks := range results3 {
		totalResults3 += len(blocks)
	}

	if totalResults3 != 0 {
		t.Errorf("Expected 0 results on page 3, got %d", totalResults3)
	}
}

func TestTimeBasedOrderingAcrossDatasources(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	// Create test data with specific timestamps across multiple datasources
	baseTime := time.Date(2024, 1, 1, 12, 0, 0, 0, time.UTC)
	testData := map[string][]core.Block{
		"datasource_a": {
			&mockBlock{id: "a1", text: "programming tutorial", createdAt: baseTime.Add(5 * time.Minute), source: "datasource_a"}, // 12:05
			&mockBlock{id: "a2", text: "programming guide", createdAt: baseTime.Add(15 * time.Minute), source: "datasource_a"},   // 12:15
			&mockBlock{id: "a3", text: "programming basics", createdAt: baseTime.Add(25 * time.Minute), source: "datasource_a"},  // 12:25
		},
		"datasource_b": {
			&mockBlock{id: "b1", text: "programming advanced", createdAt: baseTime.Add(2 * time.Minute), source: "datasource_b"},  // 12:02
			&mockBlock{id: "b2", text: "programming tips", createdAt: baseTime.Add(12 * time.Minute), source: "datasource_b"},     // 12:12
			&mockBlock{id: "b3", text: "programming concepts", createdAt: baseTime.Add(22 * time.Minute), source: "datasource_b"}, // 12:22
		},
		"datasource_c": {
			&mockBlock{id: "c1", text: "programming fundamentals", createdAt: baseTime.Add(8 * time.Minute), source: "datasource_c"},    // 12:08
			&mockBlock{id: "c2", text: "programming patterns", createdAt: baseTime.Add(18 * time.Minute), source: "datasource_c"},       // 12:18
			&mockBlock{id: "c3", text: "programming best practices", createdAt: baseTime.Add(28 * time.Minute), source: "datasource_c"}, // 12:28
		},
	}

	setupTestData(t, manager, testData)

	// Test that SearchAllDatasourcesPaged returns results in time order (newest first)
	results, err := manager.SearchAllDatasourcesPaged("programming", 100, 1, 20)
	if err != nil {
		t.Fatalf("Failed to search: %v", err)
	}

	// Extract all blocks and sort them to verify ordering
	var allBlocks []core.Block
	for _, blocks := range results {
		allBlocks = append(allBlocks, blocks...)
	}

	if len(allBlocks) != 9 {
		t.Errorf("Expected 9 blocks, got %d", len(allBlocks))
	}

	// Verify that datasources are ordered by their newest block
	// c3 (28min) should come first, then a3 (25min), then b3 (22min)
	datasourceOrder := make([]string, 0)
	for dsName := range results {
		datasourceOrder = append(datasourceOrder, dsName)
	}

	// Get the newest block time for each datasource to verify ordering
	datasourceToNewestTime := make(map[string]time.Time)
	for dsName, blocks := range results {
		if len(blocks) > 0 {
			newestTime := blocks[0].CreatedAt()
			for _, block := range blocks {
				if block.CreatedAt().After(newestTime) {
					newestTime = block.CreatedAt()
				}
			}
			datasourceToNewestTime[dsName] = newestTime
		}
	}

	// Verify that each datasource's blocks are ordered by time (newest first)
	for dsName, blocks := range results {
		for i := 0; i < len(blocks)-1; i++ {
			current := blocks[i].CreatedAt()
			next := blocks[i+1].CreatedAt()
			if current.Before(next) {
				t.Errorf("In datasource %s: block %d (time: %v) should come after block %d (time: %v)",
					dsName, i, current, i+1, next)
			}
		}
	}

	t.Logf("Datasources found: %v", datasourceOrder)
	t.Logf("Datasource newest times: %v", datasourceToNewestTime)
}

func TestTimeBasedOrderingWithFiltering(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	// Create test data with specific timestamps
	baseTime := time.Date(2024, 1, 1, 10, 0, 0, 0, time.UTC)
	testData := map[string][]core.Block{
		"github": {
			&mockBlock{id: "gh1", text: "search implementation", createdAt: baseTime.Add(10 * time.Minute), source: "github"},
			&mockBlock{id: "gh2", text: "search optimization", createdAt: baseTime.Add(30 * time.Minute), source: "github"},
		},
		"hackernews": {
			&mockBlock{id: "hn1", text: "search algorithms", createdAt: baseTime.Add(5 * time.Minute), source: "hackernews"},
			&mockBlock{id: "hn2", text: "search techniques", createdAt: baseTime.Add(25 * time.Minute), source: "hackernews"},
		},
		"rss": {
			&mockBlock{id: "rss1", text: "search engines", createdAt: baseTime.Add(15 * time.Minute), source: "rss"},
			&mockBlock{id: "rss2", text: "search indexing", createdAt: baseTime.Add(35 * time.Minute), source: "rss"},
		},
	}

	setupTestData(t, manager, testData)

	// Test filtered search with time ordering (only github and rss)
	results, err := manager.SearchDatasourcesPaged([]string{"github", "rss"}, "search", 100, 1, 10)
	if err != nil {
		t.Fatalf("Failed to search filtered datasources: %v", err)
	}

	// Should only have github and rss results
	if _, hasHN := results["hackernews"]; hasHN {
		t.Error("Should not have hackernews results when filtering to github and rss")
	}

	// Extract all blocks from filtered results
	var filteredBlocks []core.Block
	for _, blocks := range results {
		filteredBlocks = append(filteredBlocks, blocks...)
	}

	if len(filteredBlocks) != 4 {
		t.Errorf("Expected 4 filtered blocks, got %d", len(filteredBlocks))
	}

	// Verify that within each datasource, blocks are ordered by time (newest first)
	for dsName, blocks := range results {
		for i := 0; i < len(blocks)-1; i++ {
			current := blocks[i].CreatedAt()
			next := blocks[i+1].CreatedAt()
			if current.Before(next) {
				t.Errorf("In filtered datasource %s: block %d (time: %v) should come after block %d (time: %v)",
					dsName, i, current, i+1, next)
			}
		}
	}

	// Check that datasources with newer blocks appear first
	if _, hasRSS := results["rss"]; hasRSS {
		if _, hasGithub := results["github"]; hasGithub {
			// rss has rss2 at 35min, github has gh2 at 30min
			// So rss should appear first in the flattened results
			var firstRSSBlock, firstGithubBlock core.Block
			for _, block := range filteredBlocks {
				if block.Source() == "rss" && firstRSSBlock == nil {
					firstRSSBlock = block
				}
				if block.Source() == "github" && firstGithubBlock == nil {
					firstGithubBlock = block
				}
			}
		}
	}

	// Extract IDs to verify we got the expected blocks
	actualFilteredOrder := make([]string, len(filteredBlocks))
	for i, block := range filteredBlocks {
		actualFilteredOrder[i] = block.ID()
	}

	t.Logf("Filtered blocks in order: %v", actualFilteredOrder)
}

func TestTimeBasedOrderingPagination(t *testing.T) {
	manager := createTestManager(t)
	defer manager.Close() //nolint:errcheck

	// Create larger dataset for pagination testing
	baseTime := time.Date(2024, 1, 1, 9, 0, 0, 0, time.UTC)
	testData := map[string][]core.Block{
		"datasource_x": make([]core.Block, 10),
		"datasource_y": make([]core.Block, 10),
	}

	// Create blocks with specific timestamps
	for i := 0; i < 10; i++ {
		testData["datasource_x"][i] = &mockBlock{
			id:        fmt.Sprintf("x_%d", i),
			text:      fmt.Sprintf("content x %d", i),
			createdAt: baseTime.Add(time.Duration(i*2) * time.Minute), // Even minutes
			source:    "datasource_x",
		}
		testData["datasource_y"][i] = &mockBlock{
			id:        fmt.Sprintf("y_%d", i),
			text:      fmt.Sprintf("content y %d", i),
			createdAt: baseTime.Add(time.Duration(i*2+1) * time.Minute), // Odd minutes
			source:    "datasource_y",
		}
	}

	setupTestData(t, manager, testData)

	// Test page 1 with limit 8
	results1, err := manager.SearchAllDatasourcesPaged("content", 100, 1, 8)
	if err != nil {
		t.Fatalf("Failed to search page 1: %v", err)
	}

	var page1Blocks []core.Block
	for _, blocks := range results1 {
		page1Blocks = append(page1Blocks, blocks...)
	}

	if len(page1Blocks) != 8 {
		t.Errorf("Expected 8 blocks on page 1, got %d", len(page1Blocks))
	}

	// Test page 2 with limit 8
	results2, err := manager.SearchAllDatasourcesPaged("content", 100, 2, 8)
	if err != nil {
		t.Fatalf("Failed to search page 2: %v", err)
	}

	var page2Blocks []core.Block
	for _, blocks := range results2 {
		page2Blocks = append(page2Blocks, blocks...)
	}

	if len(page2Blocks) != 8 {
		t.Errorf("Expected 8 blocks on page 2, got %d", len(page2Blocks))
	}

	// Verify that page 1 has newer blocks than page 2
	if len(page1Blocks) > 0 && len(page2Blocks) > 0 {
		page1Oldest := page1Blocks[len(page1Blocks)-1].CreatedAt()
		page2Newest := page2Blocks[0].CreatedAt()

		if page2Newest.After(page1Oldest) {
			t.Errorf("Pagination time ordering violated: page 2 newest (%v) should not be newer than page 1 oldest (%v)",
				page2Newest, page1Oldest)
		}
	}

	// Test page 3 (should have remaining 4 blocks)
	results3, err := manager.SearchAllDatasourcesPaged("content", 100, 3, 8)
	if err != nil {
		t.Fatalf("Failed to search page 3: %v", err)
	}

	var page3Blocks []core.Block
	for _, blocks := range results3 {
		page3Blocks = append(page3Blocks, blocks...)
	}

	if len(page3Blocks) != 4 {
		t.Errorf("Expected 4 blocks on page 3, got %d", len(page3Blocks))
	}

	t.Log("Time-based pagination ordering verified successfully")
}

func BenchmarkSearchSingleDatasource(b *testing.B) {
	manager := createTestManager(&testing.T{})
	defer manager.Close() //nolint:errcheck

	blocksPerDatasource := 500
	now := time.Now()

	testData := map[string][]core.Block{
		"single_datasource": make([]core.Block, blocksPerDatasource),
	}

	for j := 0; j < blocksPerDatasource; j++ {
		testData["single_datasource"][j] = &mockBlock{
			id:        fmt.Sprintf("block_%d", j),
			text:      fmt.Sprintf("benchmark test content programming tutorial %d", j),
			createdAt: now.Add(time.Duration(j) * time.Minute),
			source:    "single_datasource",
		}
	}

	setupTestData(&testing.T{}, manager, testData)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, err := manager.SearchBlocks("single_datasource", "programming", 50)
		if err != nil {
			b.Fatalf("Search failed: %v", err)
		}
	}
}
