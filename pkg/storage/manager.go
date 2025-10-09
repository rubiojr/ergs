package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/db"
)

// ErrPendingMigrations is returned when there are pending migrations
var ErrPendingMigrations = fmt.Errorf("pending migrations detected")

// PendingMigrationsError wraps ErrPendingMigrations with details about which
// datasource has pending migrations and how many are pending.
type PendingMigrationsError struct {
	Datasource string // Name of the datasource with pending migrations
	Count      int    // Number of pending migrations
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

// Manager manages storage operations across multiple datasources.
// It provides a unified interface for creating, accessing, and searching
// across multiple storage backends while handling migrations and maintaining
// thread safety.
type Manager struct {
	storageDir      string
	storages        map[string]*GenericStorage
	blockPrototypes map[string]core.Block
	searchService   *SearchService
	mu              sync.RWMutex
}

// NewManager creates a new storage manager with the specified storage directory.
// It checks for pending migrations in existing databases and returns an error
// if any are found. Use NewManagerWithoutMigrationCheck for migration operations.
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

// NewManagerWithoutMigrationCheck creates a storage manager without checking for pending migrations.
// This is used by the migrate command itself to avoid circular dependencies.
func NewManagerWithoutMigrationCheck(storageDir string) *Manager {
	m := &Manager{
		storageDir:      storageDir,
		storages:        make(map[string]*GenericStorage),
		blockPrototypes: make(map[string]core.Block),
	}
	m.searchService = NewSearchService(m)
	return m
}

// GetStorage retrieves or creates a storage instance for the specified datasource.
// It uses lazy initialization and thread-safe caching to ensure each datasource
// has only one storage instance.
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

// EnsureStorageWithMigrations gets storage and ensures migrations are applied for new databases.
// For new databases, it automatically applies all pending migrations.
// For existing databases, it checks for pending migrations and returns an error if any exist.
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

// InitializeDatasourceStorage initializes storage for a datasource with the given schema.
// It ensures migrations are applied and then initializes the storage schema.
func (m *Manager) InitializeDatasourceStorage(datasourceName string, schema map[string]any) error {
	// Skip storage initialization if schema is nil or empty
	// This allows datasources like "importer" to not create their own databases
	if len(schema) == 0 {
		return nil
	}

	storage, err := m.EnsureStorageWithMigrations(datasourceName)
	if err != nil {
		return err
	}

	return storage.InitializeSchema(schema)
}

// RegisterBlockPrototype registers a block prototype for a specific datasource type.
// This prototype is used to convert generic blocks to typed blocks during searches.
func (m *Manager) RegisterBlockPrototype(datasourceName string, prototype core.Block) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.blockPrototypes[datasourceName] = prototype
}

// GetDatasourceNames returns a sorted list of all datasource names that have storage.
func (m *Manager) GetDatasourceNames() []string {
	m.mu.RLock()
	defer m.mu.RUnlock()

	names := make([]string, 0, len(m.storages))
	for name := range m.storages {
		names = append(names, name)
	}
	return names
}

// GetSearchService returns the search service for external access.
// The search service provides advanced search capabilities across all datasources.
func (m *Manager) GetSearchService() *SearchService {
	return m.searchService
}

// SearchBlocks searches for blocks within a specific datasource using the given query.
// It returns up to 'limit' blocks that match the search criteria.
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

// SearchAllDatasources returns a list of all currently loaded datasource names.
// This includes only datasources that have been accessed and have storage instances.
func (m *Manager) SearchAllDatasources() []string {
	m.mu.RLock()
	datasourceNames := make([]string, 0, len(m.storages))
	for name := range m.storages {
		datasourceNames = append(datasourceNames, name)
	}
	m.mu.RUnlock()
	return datasourceNames
}

// SearchAllDatasourcesPaged searches all datasources with pagination support.
// Returns a map of datasource names to their matching blocks.
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

// SearchAllDatasourcesPagedWithDateRange searches all datasources with date filtering and pagination.
// Results are filtered to include only blocks created between startDate and endDate (inclusive).
// Either startDate or endDate can be nil to specify an open-ended range.
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

// SearchDatasourcesPaged searches specific datasources with pagination ordered by creation time.
// Only the specified datasources will be searched, and results are returned
// as a map of datasource names to their matching blocks.
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

// SearchDatasourcesPagedWithDateRange searches specific datasources with date filtering and pagination.
// Results are ordered by creation time and filtered to include only blocks created between
// startDate and endDate (inclusive). Either date can be nil for an open-ended range.
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

// sortDatasourcesByNewestBlock sorts datasources by the creation time of their newest block.
// Returns a slice of datasource names ordered by the newest block time (newest first).

// GetStats returns storage statistics for all datasources including total blocks
// and datasource-specific metrics. The returned map includes individual datasource
// stats plus aggregate totals.
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

// Close closes all storage instances and cleans up resources.
// Should be called when the manager is no longer needed to ensure
// proper cleanup of database connections.
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

// OptimizeAll runs database optimization on all storage instances.
// This uses SQLite's PRAGMA optimize to improve query performance.
func (m *Manager) OptimizeAll() error {
	return m.OptimizeAllWithProgress(nil)
}

// OptimizeAllWithProgress runs database optimization on all storage instances with progress reporting.
// The progress callback is called before optimizing each database with the datasource name.
func (m *Manager) OptimizeAllWithProgress(progressFn func(datasource string)) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		if err := storage.Optimize(); err != nil {
			errors = append(errors, fmt.Errorf("optimizing storage %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors optimizing storages: %v", errors)
	}

	return nil
}

// AnalyzeAll runs database analysis on all storage instances.
// This updates query planner statistics to improve query performance.
func (m *Manager) AnalyzeAll() error {
	return m.AnalyzeAllWithProgress(nil)
}

// AnalyzeAllWithProgress runs database analysis on all storage instances with progress reporting.
// The progress callback is called before analyzing each database with the datasource name.
func (m *Manager) AnalyzeAllWithProgress(progressFn func(datasource string)) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		if err := storage.Analyze(); err != nil {
			errors = append(errors, fmt.Errorf("analyzing storage %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors analyzing storages: %v", errors)
	}

	return nil
}

// WALCheckpointAll performs WAL (Write-Ahead Logging) checkpoint on all storage instances.
// This flushes pending writes from the WAL to the main database file,
// which can help with backup consistency and performance.
func (m *Manager) WALCheckpointAll() error {
	return m.WALCheckpointAllWithProgress(nil)
}

// WALCheckpointAllWithProgress performs WAL checkpoint on all storage instances with progress reporting.
// The progress callback is called before checkpointing each database with the datasource name.
func (m *Manager) WALCheckpointAllWithProgress(progressFn func(datasource string)) error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	var errors []error
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		if err := storage.WALCheckpoint(); err != nil {
			errors = append(errors, fmt.Errorf("WAL checkpoint for storage %s: %w", name, err))
		}
	}

	if len(errors) > 0 {
		return fmt.Errorf("errors in WAL checkpoint: %v", errors)
	}

	return nil
}

// IntegrityCheckAll runs database integrity checks on all storage instances.
// Returns a map of datasource names to their check results (nil if healthy, error if corrupted).
func (m *Manager) IntegrityCheckAll() map[string]error {
	return m.IntegrityCheckAllWithProgress(nil)
}

// IntegrityCheckAllWithProgress runs database integrity checks on all storage instances with progress reporting.
// The progress callback is called before checking each database with the datasource name.
// Returns a map of datasource names to their check results (nil if healthy, error if corrupted).
func (m *Manager) IntegrityCheckAllWithProgress(progressFn func(datasource string)) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		results[name] = storage.IntegrityCheck()
	}

	return results
}

// FTSRebuildAll rebuilds FTS5 indexes for all storage instances.
// Returns a map of datasource names to their rebuild results (nil if successful, error if failed).
func (m *Manager) FTSRebuildAll() map[string]error {
	return m.FTSRebuildAllWithProgress(nil)
}

// FTSRebuildAllWithProgress rebuilds FTS5 indexes for all storage instances with progress reporting.
// The progress callback is called before rebuilding each database with the datasource name.
// Returns a map of datasource names to their rebuild results (nil if successful, error if failed).
func (m *Manager) FTSRebuildAllWithProgress(progressFn func(datasource string)) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		results[name] = storage.FTSRebuild()
	}

	return results
}

// VacuumAll runs VACUUM on all storage instances.
// Returns a map of datasource names to their vacuum results (nil if successful, error if failed).
func (m *Manager) VacuumAll() map[string]error {
	return m.VacuumAllWithProgress(nil)
}

// VacuumAllWithProgress runs VACUUM on all storage instances with progress reporting.
// The progress callback is called before vacuuming each database with the datasource name.
// Returns a map of datasource names to their vacuum results (nil if successful, error if failed).
func (m *Manager) VacuumAllWithProgress(progressFn func(datasource string)) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		results[name] = storage.Vacuum()
	}

	return results
}

// FTSIntegrityCheckAll performs deep FTS5-specific integrity checks on all storage instances.
// Returns a map of datasource names to their check results (nil if healthy, error if corrupted).
func (m *Manager) FTSIntegrityCheckAll() map[string]error {
	return m.FTSIntegrityCheckAllWithProgress(nil)
}

// FTSIntegrityCheckAllWithProgress performs deep FTS5-specific integrity checks with progress reporting.
// The progress callback is called before checking each database with the datasource name.
// Returns a map of datasource names to their check results (nil if healthy, error if corrupted).
func (m *Manager) FTSIntegrityCheckAllWithProgress(progressFn func(datasource string)) map[string]error {
	m.mu.RLock()
	defer m.mu.RUnlock()

	results := make(map[string]error)
	for name, storage := range m.storages {
		if progressFn != nil {
			progressFn(name)
		}
		results[name] = storage.FTSIntegrityCheck()
	}

	return results
}

// checkPendingMigrations checks all existing databases for pending migrations.
// Returns a PendingMigrationsError if any database has pending migrations.
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
