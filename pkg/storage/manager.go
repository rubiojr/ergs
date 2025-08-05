package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/db"
)

// ErrPendingMigrations is returned when there are pending migrations
var ErrPendingMigrations = fmt.Errorf("pending migrations detected")

// PendingMigrationsError wraps ErrPendingMigrations with details
type PendingMigrationsError struct {
	Datasource string
	Count      int
}

func (e *PendingMigrationsError) Error() string {
	return fmt.Sprintf("database '%s' has %d pending migrations. Run 'ergs migrate' first", e.Datasource, e.Count)
}

func (e *PendingMigrationsError) Is(target error) bool {
	return target == ErrPendingMigrations
}

func (e *PendingMigrationsError) Unwrap() error {
	return ErrPendingMigrations
}

type Manager struct {
	storageDir      string
	storages        map[string]*GenericStorage
	blockPrototypes map[string]core.Block
	searchService   *SearchService
	mu              sync.RWMutex
}

func NewManager(storageDir string) (*Manager, error) {
	manager := &Manager{
		storageDir:      storageDir,
		storages:        make(map[string]*GenericStorage),
		blockPrototypes: make(map[string]core.Block),
	}
	manager.searchService = NewSearchService(manager)

	// Check for pending migrations in existing databases
	if err := manager.checkPendingMigrations(); err != nil {
		return nil, err
	}

	return manager, nil
}

// NewManagerWithoutMigrationCheck creates a storage manager without checking for pending migrations
// This is used by the migrate command itself to avoid circular dependencies
func NewManagerWithoutMigrationCheck(storageDir string) *Manager {
	m := &Manager{
		storageDir:      storageDir,
		storages:        make(map[string]*GenericStorage),
		blockPrototypes: make(map[string]core.Block),
	}
	m.searchService = NewSearchService(m)
	return m
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

// EnsureStorageWithMigrations gets storage and ensures migrations are applied for new databases
func (m *Manager) EnsureStorageWithMigrations(datasourceName string) (*GenericStorage, error) {
	dbPath := filepath.Join(m.storageDir, fmt.Sprintf("%s.db", datasourceName))

	// Check if database file exists before creating storage
	dbExists := true
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		dbExists = false
	}

	storage, err := m.GetStorage(datasourceName)
	if err != nil {
		return nil, err
	}

	// For new databases, apply migrations automatically
	// For existing databases, check for pending migrations and return error if any exist
	if !dbExists {
		migrationManager := db.NewMigrationManager(storage.GetDB())
		if err := migrationManager.ApplyPendingMigrations(); err != nil {
			return nil, fmt.Errorf("applying migrations for new database %s: %w", datasourceName, err)
		}
	} else {
		// Check for pending migrations on existing databases
		migrationManager := db.NewMigrationManager(storage.GetDB())

		// Ensure migrations table exists before checking
		if err := migrationManager.EnsureMigrationsTable(); err != nil {
			return nil, fmt.Errorf("ensuring migrations table for %s: %w", datasourceName, err)
		}

		pending, err := migrationManager.GetPendingMigrations()
		if err != nil {
			return nil, fmt.Errorf("checking pending migrations for %s: %w", datasourceName, err)
		}
		if len(pending) > 0 {
			return nil, &PendingMigrationsError{
				Datasource: datasourceName,
				Count:      len(pending),
			}
		}
	}

	return storage, nil
}

func (m *Manager) InitializeDatasourceStorage(datasourceName string, schema map[string]any) error {
	storage, err := m.EnsureStorageWithMigrations(datasourceName)
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

// GetSearchService returns the search service for external access
func (m *Manager) GetSearchService() *SearchService {
	return m.searchService
}

func (m *Manager) SearchBlocks(datasourceName, query string, limit int) ([]core.Block, error) {
	params := SearchParams{
		Query:             query,
		DatasourceFilters: []string{datasourceName},
		Page:              1,
		Limit:             limit,
	}

	results, err := m.searchService.Search(params)
	if err != nil {
		return nil, err
	}

	// Flatten results from all datasources
	var allBlocks []core.Block
	for _, blocks := range results.Results {
		allBlocks = append(allBlocks, blocks...)
	}

	return allBlocks, nil
}

func (m *Manager) convertBlocksToProperTypes(blocks []core.Block) ([]core.Block, error) {
	convertedBlocks := make([]core.Block, len(blocks))
	for i, block := range blocks {
		genericBlock, ok := block.(*core.GenericBlock)
		if !ok {
			convertedBlocks[i] = block
			continue
		}

		// Get datasource type from the generic block
		datasourceType := genericBlock.DSType()

		m.mu.RLock()
		prototype, exists := m.blockPrototypes[datasourceType]
		m.mu.RUnlock()

		if !exists {
			// No prototype registered, return block as-is
			convertedBlocks[i] = block
			continue
		}

		// Extract source from metadata
		source := genericBlock.Source()
		metadata := genericBlock.Metadata()
		if sourceVal, exists := metadata["source"]; exists {
			if sourceStr, ok := sourceVal.(string); ok {
				source = sourceStr
			}
		}
		if source == "" {
			source = datasourceType // fallback
		}

		// Use the prototype's Factory method to create the proper type
		convertedBlocks[i] = prototype.Factory(genericBlock, source)
	}
	return convertedBlocks, nil
}

func (m *Manager) SearchAllDatasources() []string {
	m.mu.RLock()
	datasourceNames := make([]string, 0, len(m.storages))
	for name := range m.storages {
		datasourceNames = append(datasourceNames, name)
	}
	m.mu.RUnlock()
	return datasourceNames
}

func (m *Manager) SearchAllDatasourcesPaged(query string, limit, page, pageSize int) (map[string][]core.Block, error) {
	params := SearchParams{
		Query: query,
		Page:  page,
		Limit: pageSize,
	}

	results, err := m.searchService.Search(params)
	if err != nil {
		return nil, err
	}

	return results.Results, nil
}

// SearchAllDatasourcesPagedWithDateRange searches all datasources with date filtering and pagination
func (m *Manager) SearchAllDatasourcesPagedWithDateRange(query string, limit, page, pageSize int, startDate, endDate *time.Time) (map[string][]core.Block, error) {
	params := SearchParams{
		Query:     query,
		Page:      page,
		Limit:     pageSize,
		StartDate: startDate,
		EndDate:   endDate,
	}

	results, err := m.searchService.Search(params)
	if err != nil {
		return nil, err
	}

	return results.Results, nil
}

// SearchDatasourcesPaged searches specific datasources with pagination ordered by creation time
func (m *Manager) SearchDatasourcesPaged(datasourceNames []string, query string, limit, page, pageSize int) (map[string][]core.Block, error) {
	params := SearchParams{
		Query:             query,
		DatasourceFilters: datasourceNames,
		Page:              page,
		Limit:             pageSize,
	}

	results, err := m.searchService.Search(params)
	if err != nil {
		return nil, err
	}

	return results.Results, nil
}

// SearchDatasourcesPagedWithDateRange searches specific datasources with date filtering and pagination ordered by creation time
func (m *Manager) SearchDatasourcesPagedWithDateRange(datasourceNames []string, query string, limit, page, pageSize int, startDate, endDate *time.Time) (map[string][]core.Block, error) {
	params := SearchParams{
		Query:             query,
		DatasourceFilters: datasourceNames,
		Page:              page,
		Limit:             pageSize,
		StartDate:         startDate,
		EndDate:           endDate,
	}

	results, err := m.searchService.Search(params)
	if err != nil {
		return nil, err
	}

	return results.Results, nil
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

// checkPendingMigrations checks all existing databases for pending migrations
func (m *Manager) checkPendingMigrations() error {
	// Check if storage directory exists
	if _, err := os.Stat(m.storageDir); os.IsNotExist(err) {
		return nil // No storage directory means no databases to check
	}

	// Read all .db files in storage directory
	entries, err := os.ReadDir(m.storageDir)
	if err != nil {
		return fmt.Errorf("reading storage directory: %w", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() && filepath.Ext(entry.Name()) == ".db" {
			datasourceName := entry.Name()[:len(entry.Name())-3] // Remove .db extension
			dbPath := filepath.Join(m.storageDir, entry.Name())

			// Open database connection to check migrations
			storage, err := NewGenericStorage(dbPath, datasourceName)
			if err != nil {
				continue // Skip databases we can't open
			}

			migrationManager := db.NewMigrationManager(storage.GetDB())
			pending, err := migrationManager.GetPendingMigrations()
			if err := storage.Close(); err != nil {
				// Log close error but continue checking other databases
				fmt.Printf("Warning: failed to close storage during migration check: %v\n", err)
			}

			if err != nil {
				continue // Skip databases we can't check
			}

			if len(pending) > 0 {
				return &PendingMigrationsError{
					Datasource: datasourceName,
					Count:      len(pending),
				}
			}
		}
	}

	return nil
}
