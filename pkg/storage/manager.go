package storage

import (
	"fmt"
	"path/filepath"
	"sync"

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

func (m *Manager) SearchAllDatasources(query string, limit int) (map[string][]core.Block, error) {
	m.mu.RLock()
	datasourceNames := make([]string, 0, len(m.storages))
	for name := range m.storages {
		datasourceNames = append(datasourceNames, name)
	}
	m.mu.RUnlock()

	results := make(map[string][]core.Block)
	for _, name := range datasourceNames {
		blocks, err := m.SearchBlocks(name, query, limit)
		if err != nil {
			return nil, fmt.Errorf("searching %s: %w", name, err)
		}
		if len(blocks) > 0 {
			results[name] = blocks
		}
	}

	return results, nil
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
