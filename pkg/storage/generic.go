package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

// GenericStorage provides a generic SQLite-based storage implementation for blocks.
// It uses FTS5 (Full-Text Search) for efficient text searching and includes
// performance optimizations for handling large datasets.
type GenericStorage struct {
	db             *sql.DB
	datasourceName string
}

// NewGenericStorage creates a new GenericStorage instance with the specified database path
// and datasource name. It opens a SQLite database connection and applies performance
// optimizations including WAL mode, memory temp storage, and mmap configuration.
func NewGenericStorage(dbPath, datasourceName string) (*GenericStorage, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Apply performance pragmas
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 30000",
		"PRAGMA cache_size = -64000",   // 64MB cache
		"PRAGMA mmap_size = 268435456", // 256MB mmap
		"PRAGMA optimize",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			return nil, fmt.Errorf("applying pragma %q: %w", pragma, err)
		}
	}

	storage := &GenericStorage{
		db:             db,
		datasourceName: datasourceName,
	}

	return storage, nil
}

// Close closes the underlying database connection and releases associated resources.
// Should be called when the storage instance is no longer needed.
func (s *GenericStorage) Close() error {
	return s.db.Close()
}

// GetDB returns the underlying database connection for migrations and direct database access.
// This is primarily used by the migration system to apply schema changes.
func (s *GenericStorage) GetDB() *sql.DB {
	return s.db
}

// InitializeSchema initializes the storage schema with the provided configuration.
// This is a no-op in the current implementation as datasource-specific tables
// are not used - all data is stored in the unified blocks table.
func (s *GenericStorage) InitializeSchema(schema map[string]any) error {
	// Schema functionality removed as datasource-specific tables are not used
	return nil
}

// StoreBlock stores a single block in the database with the specified datasource type.
// This is a convenience method that calls StoreBlocks with a single-element slice.
func (s *GenericStorage) StoreBlock(block core.Block, datasourceType string) error {
	return s.StoreBlocks([]core.Block{block}, datasourceType)
}

// StoreBlocks stores multiple blocks in the database using a single transaction.
// Each block is stored in both the main blocks table and the FTS (Full-Text Search) table.
// The operation is atomic - either all blocks are stored or none are.
func (s *GenericStorage) StoreBlocks(blocks []core.Block, datasourceType string) error {
	if len(blocks) == 0 {
		return nil
	}

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				fmt.Printf("Warning: failed to rollback transaction: %v\n", err)
			}
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO blocks (id, text, created_at, source, datasource, metadata, hostname)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing statement: %w", err)
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			fmt.Printf("Warning: failed to close statement: %v\n", err)
		}
	}()

	ftsStmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO blocks_fts (rowid, text, source, datasource, metadata, hostname)
		VALUES ((SELECT rowid FROM blocks WHERE id = ?), ?, ?, ?, ?, ?)
	`)
	if err != nil {
		return fmt.Errorf("preparing FTS statement: %w", err)
	}
	defer func() {
		if err := ftsStmt.Close(); err != nil {
			fmt.Printf("Warning: failed to close FTS statement: %v\n", err)
		}
	}()

	for _, block := range blocks {
		// Convert to GenericBlock to get hostname information
		genericBlock := core.ToGenericBlockWithAutoHostname(block)

		metadataJSON, err := json.Marshal(genericBlock.Metadata())
		if err != nil {
			return fmt.Errorf("marshaling metadata for block %s: %w", genericBlock.ID(), err)
		}

		// Insert into main blocks table
		_, err = stmt.Exec(
			genericBlock.ID(),
			genericBlock.Text(),
			genericBlock.CreatedAt(),
			genericBlock.Source(),
			datasourceType,
			string(metadataJSON),
			genericBlock.Hostname(),
		)
		if err != nil {
			return fmt.Errorf("inserting block %s: %w", genericBlock.ID(), err)
		}

		// Insert into FTS table using the original text
		_, err = ftsStmt.Exec(
			genericBlock.ID(),
			genericBlock.Text(),
			genericBlock.Source(),
			datasourceType,
			string(metadataJSON),
			genericBlock.Hostname(),
		)
		if err != nil {
			return fmt.Errorf("inserting block %s into FTS: %w", genericBlock.ID(), err)
		}
	}

	err = tx.Commit()
	if err == nil {
		committed = true
	}
	return err
}

// GetBlocksSince retrieves all blocks created after the specified time.
// Results are ordered by creation time in descending order (newest first).
func (s *GenericStorage) GetBlocksSince(since time.Time) ([]core.Block, error) {
	query := `
		SELECT id, text, created_at, source, datasource, metadata, hostname
		FROM blocks
		WHERE created_at > ?
		ORDER BY created_at DESC
	`

	rows, err := s.db.Query(query, since)
	if err != nil {
		return nil, fmt.Errorf("querying blocks: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Warning: failed to close rows: %v\n", err)
		}
	}()

	var blocks []core.Block
	for rows.Next() {
		var id, text, source, datasourceType, metadataStr string
		var hostname sql.NullString
		var createdAt time.Time

		err = rows.Scan(&id, &text, &createdAt, &source, &datasourceType, &metadataStr, &hostname)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			return nil, fmt.Errorf("unmarshaling metadata for block %s: %w", id, err)
		}

		hostnameStr := ""
		if hostname.Valid {
			hostnameStr = hostname.String
		}

		block := core.NewGenericBlockWithHostname(id, text, source, datasourceType, hostnameStr, createdAt, metadata)
		blocks = append(blocks, block)
	}

	return blocks, rows.Err()
}

// GetStats returns statistics about the stored data including total block count,
// oldest block timestamp, and newest block timestamp. The returned map contains
// "total_blocks", "oldest_block", and "newest_block" keys.
func (s *GenericStorage) GetStats() (map[string]interface{}, error) {
	stats := make(map[string]interface{})

	var totalBlocks int
	err := s.db.QueryRow("SELECT COUNT(*) FROM blocks").Scan(&totalBlocks)
	if err != nil {
		return nil, fmt.Errorf("counting blocks: %w", err)
	}
	stats["total_blocks"] = totalBlocks

	var oldestBlockStr, newestBlockStr sql.NullString
	err = s.db.QueryRow("SELECT MIN(created_at), MAX(created_at) FROM blocks").Scan(&oldestBlockStr, &newestBlockStr)
	if err != nil && err != sql.ErrNoRows {
		return nil, fmt.Errorf("getting block date range: %w", err)
	}

	if oldestBlockStr.Valid && newestBlockStr.Valid {
		// Try RFC3339 format first (ncruces driver), then fall back to old format (mattn driver)
		oldestBlock, err := time.Parse(time.RFC3339, oldestBlockStr.String)
		if err != nil {
			oldestBlock, err = time.Parse("2006-01-02 15:04:05-07:00", oldestBlockStr.String)
			if err != nil {
				return nil, fmt.Errorf("parsing oldest block time: %w", err)
			}
		}
		newestBlock, err := time.Parse(time.RFC3339, newestBlockStr.String)
		if err != nil {
			newestBlock, err = time.Parse("2006-01-02 15:04:05-07:00", newestBlockStr.String)
			if err != nil {
				return nil, fmt.Errorf("parsing newest block time: %w", err)
			}
		}
		stats["oldest_block"] = oldestBlock
		stats["newest_block"] = newestBlock
	}

	return stats, nil
}

// GetLastFetchTime retrieves the last recorded fetch time for this datasource.
// Returns zero time if no fetch time has been recorded yet.
func (s *GenericStorage) GetLastFetchTime() (time.Time, error) {
	var lastFetchStr string
	err := s.db.QueryRow(`
		SELECT value FROM fetch_metadata WHERE key = 'last_fetch'
	`).Scan(&lastFetchStr)

	if err == sql.ErrNoRows {
		return time.Time{}, nil
	}
	if err != nil {
		return time.Time{}, err
	}

	return time.Parse(time.RFC3339, lastFetchStr)
}

// ExecuteQuery executes a SQL query with optional parameters and returns the result rows.
// The caller is responsible for closing the returned rows.
func (s *GenericStorage) ExecuteQuery(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

// ExecuteStatement executes a SQL statement (INSERT, UPDATE, DELETE) with optional parameters.
// Returns the result containing information about rows affected.
func (s *GenericStorage) ExecuteStatement(query string, args ...interface{}) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

// Optimize runs SQLite's optimization routine to improve query performance.
// This analyzes the database and updates internal statistics.
func (s *GenericStorage) Optimize() error {
	_, err := s.db.Exec("PRAGMA optimize")
	return err
}

// Analyze updates the query planner statistics by analyzing the database tables.
// This can improve query performance by providing better statistics to the optimizer.
func (s *GenericStorage) Analyze() error {
	_, err := s.db.Exec("ANALYZE")
	return err
}

// Vacuum reclaims unused space in the database file and defragments the database.
// This operation can be slow on large databases but reduces file size.
func (s *GenericStorage) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	return err
}

// WALCheckpoint performs a WAL (Write-Ahead Logging) checkpoint operation.
// This flushes pending writes from the WAL to the main database file and truncates the WAL.
func (s *GenericStorage) WALCheckpoint() error {
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// IntegrityCheck runs SQLite's integrity check on the database including FTS5-specific checks.
// Returns nil if the database is healthy, or an error describing the corruption.
func (s *GenericStorage) IntegrityCheck() error {
	// First run standard integrity check
	var result string
	err := s.db.QueryRow("PRAGMA integrity_check").Scan(&result)
	if err != nil {
		return fmt.Errorf("running integrity check: %w", err)
	}

	if result != "ok" {
		return fmt.Errorf("integrity check failed: %s", result)
	}

	// Check if FTS table exists
	var ftsExists int
	err = s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='blocks_fts'").Scan(&ftsExists)
	if err != nil {
		return fmt.Errorf("checking for FTS table: %w", err)
	}

	if ftsExists == 0 {
		// No FTS table, nothing more to check
		return nil
	}

	// Try a simple FTS query to detect query-time corruption
	rows, err := s.db.Query("SELECT rowid FROM blocks_fts LIMIT 1")
	if err != nil {
		return fmt.Errorf("FTS query test failed (possible corruption): %w", err)
	}
	if err := rows.Close(); err != nil {
		return fmt.Errorf("closing FTS query rows: %w", err)
	}

	return nil
}

// FTSIntegrityCheck performs deep FTS5-specific integrity checks.
// This can detect corruption that standard integrity_check misses.
// Uses both FTS5's built-in integrity-check command and actual queries.
func (s *GenericStorage) FTSIntegrityCheck() error {
	// Check if FTS table exists
	var ftsExists int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='blocks_fts'").Scan(&ftsExists)
	if err != nil {
		return fmt.Errorf("checking for FTS table: %w", err)
	}

	if ftsExists == 0 {
		return fmt.Errorf("FTS table 'blocks_fts' does not exist")
	}

	// Run FTS5's built-in integrity check command
	// Note: This is a special FTS5 command, not a data insertion
	_, err = s.db.Exec("INSERT INTO blocks_fts(blocks_fts) VALUES('integrity-check')")
	if err != nil {
		return fmt.Errorf("FTS integrity-check command failed: %w", err)
	}

	// Try a COUNT query - this often reveals corruption
	var count int
	err = s.db.QueryRow("SELECT COUNT(*) FROM blocks_fts").Scan(&count)
	if err != nil {
		return fmt.Errorf("FTS COUNT query failed (possible corruption): %w", err)
	}

	// Test various FTS queries that retrieve actual content from blocks table.
	// This is critical because with external content tables (content='blocks'),
	// the FTS index can reference rows that no longer exist in the blocks table.
	// These queries force FTS to fetch data from blocks, revealing sync issues.

	testQueries := []struct {
		name  string
		query string
	}{
		{"simple MATCH", "SELECT rowid, text FROM blocks_fts WHERE blocks_fts MATCH 'a' LIMIT 100"},
		{"phrase query", "SELECT rowid, text FROM blocks_fts WHERE blocks_fts MATCH '\"the\"' LIMIT 100"},
		{"two-word phrase", "SELECT rowid, text FROM blocks_fts WHERE blocks_fts MATCH '\"the a\"' LIMIT 100"},
		{"common words", "SELECT rowid, text FROM blocks_fts WHERE blocks_fts MATCH '\"in the\"' LIMIT 100"},
	}

	for _, test := range testQueries {
		rows, err := s.db.Query(test.query)
		if err != nil {
			return fmt.Errorf("FTS %s failed (possible corruption): %w", test.name, err)
		}

		// Iterate through results and try to read the text column
		// This forces SQLite to access the blocks table via the content_rowid
		for rows.Next() {
			var rowid int64
			var text string
			if err := rows.Scan(&rowid, &text); err != nil {
				_ = rows.Close()
				return fmt.Errorf("FTS %s scan failed at rowid (missing row in blocks table?): %w", test.name, err)
			}
		}

		if err := rows.Err(); err != nil {
			_ = rows.Close()
			return fmt.Errorf("FTS %s iteration failed (possible corruption): %w", test.name, err)
		}
		if err := rows.Close(); err != nil {
			return fmt.Errorf("closing FTS %s query rows: %w", test.name, err)
		}
	}

	return nil
}

// FTSRebuild rebuilds the FTS5 full-text search index from the blocks table.
// This can fix corrupted FTS indexes and should be used after data recovery.
func (s *GenericStorage) FTSRebuild() error {
	// First check if the FTS table exists
	var count int
	err := s.db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name='blocks_fts'").Scan(&count)
	if err != nil {
		return fmt.Errorf("checking for FTS table: %w", err)
	}

	if count == 0 {
		return fmt.Errorf("FTS table 'blocks_fts' does not exist")
	}

	// Rebuild the FTS index
	_, err = s.db.Exec("INSERT INTO blocks_fts(blocks_fts) VALUES('rebuild')")
	if err != nil {
		return fmt.Errorf("rebuilding FTS index: %w", err)
	}

	// Optimize the FTS index
	_, err = s.db.Exec("INSERT INTO blocks_fts(blocks_fts) VALUES('optimize')")
	if err != nil {
		return fmt.Errorf("optimizing FTS index: %w", err)
	}

	return nil
}
