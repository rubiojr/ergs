package storage

import (
	"fmt"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

type Manager struct {
	storageDir      string
	storages        map[string]*GenericStorage
	blockPrototypes map[string]core.Block
	mu              sync.RWMutex
}

func NewManager(storageDir string) *Manager {
	return &Manager{
		storageDir:      storageDir,
		storages:        make(map[string]*GenericStorage),
		blockPrototypes: make(map[string]core.Block),
	}
}

func (m *Manager) GetStorage(datasourceName string) (*GenericStorage, error) {
	m.mu.RLock()
	storage, exists := m.storages[datasourceName]
	m.mu.RUnlock()

	if exists {
		return storage, nil
	}

	m.mu.Lock()
	defer m.mu.Unlock()

	if storage, exists := m.storages[datasourceName]; exists {
		return storage, nil
	}

	dbPath := filepath.Join(m.storageDir, fmt.Sprintf("%s.db", datasourceName))
	storage, err := NewGenericStorage(dbPath, datasourceName)
	if err != nil {
		return nil, fmt.Errorf("creating storage for %s: %w", datasourceName, err)
	}

	m.storages[datasourceName] = storage
	return storage, nil
}

func (m *Manager) InitializeDatasourceStorage(datasourceName string, schema map[string]any) error {
	storage, err := m.GetStorage(datasourceName)
	if err != nil {
		return err
	}

	return storage.InitializeSchema(schema)
}

func (m *Manager) RegisterBlockPrototype(datasourceName string, prototype core.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockPrototypes[datasourceName] = prototype
}

func (m *Manager) SearchBlocks(datasourceName, query string, limit int) ([]core.Block, error) {
	storage, err := m.GetStorage(datasourceName)
	if err != nil {
		return nil, err
	}

	blocks, err := storage.SearchBlocks(query, limit)
	if err != nil {
		return nil, err
	}

	// Convert blocks to their proper types using registered factories
	return m.convertBlocksToProperTypes(datasourceName, blocks), nil
}

// SearchBlocksByTime searches blocks and orders them strictly by creation time (newest first)
func (m *Manager) SearchBlocksByTime(datasourceName, query string, limit int) ([]core.Block, error) {
	storage, err := m.GetStorage(datasourceName)
	if err != nil {
		return nil, err
	}

	blocks, err := storage.SearchBlocksByTime(query, limit)
	if err != nil {
		return nil, err
	}

	// Convert blocks to their proper types using registered factories
	return m.convertBlocksToProperTypes(datasourceName, blocks), nil
}

func (m *Manager) convertBlocksToProperTypes(datasourceName string, blocks []core.Block) []core.Block {
	m.mu.RLock()
	prototype, exists := m.blockPrototypes[datasourceName]
	m.mu.RUnlock()

	if !exists {
		// No prototype registered, return blocks as-is
		return blocks
	}

	convertedBlocks := make([]core.Block, len(blocks))
	for i, block := range blocks {
		// Convert block to GenericBlock and extract source from metadata
		genericBlock, ok := block.(*core.GenericBlock)
		var source string

		if !ok {
			// If it's not a GenericBlock, create one
			metadata := block.Metadata()
			if sourceVal, exists := metadata["source"]; exists {
				if sourceStr, ok := sourceVal.(string); ok {
					source = sourceStr
				}
			}
			if source == "" {
				source = datasourceName // fallback
			}
			genericBlock = core.NewGenericBlock(
				block.ID(),
				block.Text(),
				block.Source(),
				"unknown", // datasource type - will be set properly during storage
				block.CreatedAt(),
				metadata,
			)
		} else {
			// Extract source from metadata
			metadata := genericBlock.Metadata()
			if sourceVal, exists := metadata["source"]; exists {
				if sourceStr, ok := sourceVal.(string); ok {
					source = sourceStr
				}
			}
			if source == "" {
				source = datasourceName // fallback
			}
		}

		// Use the prototype's Factory method to create the proper type
		convertedBlocks[i] = prototype.Factory(genericBlock, source)
	}
	return convertedBlocks
}

type searchResult struct {
	datasource string
	blocks     []core.Block
	err        error
}

func (m *Manager) searchDatasourcesInParallel(datasourceNames []string, query string, requestLimit int) (map[string][]core.Block, error) {
	resultCh := make(chan searchResult, len(datasourceNames))
	var wg sync.WaitGroup

	for _, name := range datasourceNames {
		wg.Add(1)
		go func(datasourceName string) {
			defer wg.Done()
			blocks, err := m.SearchBlocksByTime(datasourceName, query, requestLimit)
			resultCh <- searchResult{
				datasource: datasourceName,
				blocks:     blocks,
				err:        err,
			}
		}(name)
	}

	go func() {
		wg.Wait()
		close(resultCh)
	}()

	allResults := make(map[string][]core.Block)
	for result := range resultCh {
		if result.err != nil {
			return nil, fmt.Errorf("searching %s: %w", result.datasource, result.err)
		}
		if len(result.blocks) > 0 {
			allResults[result.datasource] = result.blocks
		}
	}

	return allResults, nil
}

func (m *Manager) SearchAllDatasources(query string, limit int) (map[string][]core.Block, error) {
	return m.SearchAllDatasourcesPaged(query, limit, 1, limit)
}

func (m *Manager) SearchAllDatasourcesPaged(query string, limit, page, pageSize int) (map[string][]core.Block, error) {
	m.mu.RLock()
	datasourceNames := make([]string, 0, len(m.storages))
	for name := range m.storages {
		datasourceNames = append(datasourceNames, name)
	}
	m.mu.RUnlock()

	return m.SearchDatasourcesPaged(datasourceNames, query, limit, page, pageSize)
}

// SearchDatasourcesPaged searches specific datasources with pagination ordered by creation time
func (m *Manager) SearchDatasourcesPaged(datasourceNames []string, query string, limit, page, pageSize int) (map[string][]core.Block, error) {
	// Filter to only include datasources that actually exist
	m.mu.RLock()
	validDatasources := make([]string, 0, len(datasourceNames))
	for _, name := range datasourceNames {
		if _, exists := m.storages[name]; exists {
			validDatasources = append(validDatasources, name)
		}
	}
	m.mu.RUnlock()

	if len(validDatasources) == 0 {
		return make(map[string][]core.Block), nil
	}
	// Get enough results to support paging
	requestLimit := page * pageSize
	allResults, err := m.searchDatasourcesInParallel(validDatasources, query, requestLimit)
	if err != nil {
		return nil, err
	}

	// Sort datasources by the creation time of their newest block
	sortedDatasources := m.sortDatasourcesByNewestBlock(allResults)

	// Flatten results in datasource order (ordered by newest block per datasource)
	var allBlocks []core.Block
	var blockToDatasource []string

	for _, dsName := range sortedDatasources {
		if blocks, exists := allResults[dsName]; exists {
			for _, block := range blocks {
				allBlocks = append(allBlocks, block)
				blockToDatasource = append(blockToDatasource, dsName)
			}
		}
	}

	// Apply pagination to the flattened list
	startIndex := (page - 1) * pageSize
	endIndex := startIndex + pageSize

	if startIndex >= len(allBlocks) {
		return make(map[string][]core.Block), nil
	}

	if endIndex > len(allBlocks) {
		endIndex = len(allBlocks)
	}

	// Group the paginated results back by datasource
	results := make(map[string][]core.Block)
	for i := startIndex; i < endIndex; i++ {
		dsName := blockToDatasource[i]
		block := allBlocks[i]
		results[dsName] = append(results[dsName], block)
	}

	return results, nil
}

// sortDatasourcesByNewestBlock sorts datasources by the creation time of their newest block
func (m *Manager) sortDatasourcesByNewestBlock(datasourceResults map[string][]core.Block) []string {
	type datasourceWithTime struct {
		name       string
		newestTime time.Time
	}

	var datasources []datasourceWithTime

	for dsName, blocks := range datasourceResults {
		if len(blocks) > 0 {
			// Find the newest block in this datasource (blocks should already be sorted by time)
			newestTime := blocks[0].CreatedAt()
			for _, block := range blocks {
				if block.CreatedAt().After(newestTime) {
					newestTime = block.CreatedAt()
				}
			}
			datasources = append(datasources, datasourceWithTime{
				name:       dsName,
				newestTime: newestTime,
			})
		}
	}

	// Sort datasources by newest block time (newest first)
	sort.Slice(datasources, func(i, j int) bool {
		return datasources[i].newestTime.After(datasources[j].newestTime)
	})

	result := make([]string, len(datasources))
	for i, ds := range datasources {
		result[i] = ds.name
	}

	return result
}

func (m *Manager) GetStats() (map[string]interface{}, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	stats := make(map[string]interface{})
	totalBlocks := 0

	for datasourceName, storage := range m.storages {
		dsStats, err := storage.GetStats()
		if err != nil {
			return nil, fmt.Errorf("getting stats for %s: %w", datasourceName, err)
		}

		stats[datasourceName] = dsStats
		if blockCount, ok := dsStats["total_blocks"].(int); ok {
			totalBlocks += blockCount
		}
	}

	stats["total_blocks"] = totalBlocks
	stats["total_datasources"] = len(m.storages)

	return stats, nil
}

func (m *Manager) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	var errors []error
	for name, storage := range m.storages {
		if err := storage.Close(); err != nil {
			errors = append(errors, fmt.Errorf("closing storage %s: %w", name, err))
		}
	}

	m.storages = make(map[string]*GenericStorage)

	if len(errors) > 0 {
		return fmt.Errorf("errors closing storages: %v", errors)
	}

	return nil
}

func (m *Manager) OptimizeAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, storage := range m.storages {
		if err := storage.Optimize(); err != nil {
			errors = append(errors, fmt.Errorf("optimizing storage %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors optimizing storages: %v", errors)
	}

	return nil
}

func (m *Manager) AnalyzeAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, storage := range m.storages {
		if err := storage.Analyze(); err != nil {
			errors = append(errors, fmt.Errorf("analyzing storage %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors analyzing storages: %v", errors)
	}

	return nil
}

func (m *Manager) WALCheckpointAll() error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, storage := range m.storages {
		if err := storage.WALCheckpoint(); err != nil {
			errors = append(errors, fmt.Errorf("WAL checkpoint for storage %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors in WAL checkpoint: %v", errors)
	}

	return nil
}
