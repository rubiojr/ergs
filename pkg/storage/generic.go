package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

type GenericStorage struct {
	db             *sql.DB
	datasourceName string
}

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
		"PRAGMA cache_size = -64000", // 64MB cache
		"PRAGMA temp_store = memory",
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

func (s *GenericStorage) Close() error {
	return s.db.Close()
}

// GetDB returns the underlying database connection for migrations
func (s *GenericStorage) GetDB() *sql.DB {
	return s.db
}

func (s *GenericStorage) InitializeSchema(schema map[string]any) error {
	// Schema functionality removed as datasource-specific tables are not used
	return nil
}

func (s *GenericStorage) StoreBlock(block core.Block, datasourceType string) error {
	return s.StoreBlocks([]core.Block{block}, datasourceType)
}

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

func (s *GenericStorage) SearchBlocks(query string, limit int) ([]core.Block, error) {
	var sqlQuery string
	var args []interface{}

	if query != "" {
		// Escape FTS5 query for special characters
		escapedQuery := escapeFTS5Query(query)
		sqlQuery = `
			SELECT b.id, b.text, b.created_at, b.source, b.datasource, b.metadata, b.hostname
			FROM blocks b
			JOIN blocks_fts fts ON b.rowid = fts.rowid
			WHERE blocks_fts MATCH ?
			ORDER BY bm25(blocks_fts), b.created_at DESC
			LIMIT ?`
		args = []interface{}{escapedQuery, limit}
	} else {
		sqlQuery = `
			SELECT id, text, created_at, source, datasource, metadata, hostname
			FROM blocks
			ORDER BY created_at DESC
			LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := s.db.Query(sqlQuery, args...)
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

// SearchBlocksByTime searches blocks and orders them strictly by creation time (newest first)
func (s *GenericStorage) SearchBlocksByTime(query string, limit int) ([]core.Block, error) {
	var sqlQuery string
	var args []interface{}

	if query != "" {
		// Escape FTS5 query for special characters
		escapedQuery := escapeFTS5Query(query)
		sqlQuery = `
			SELECT b.id, b.text, b.created_at, b.source, b.datasource, b.metadata, b.hostname
			FROM blocks b
			JOIN blocks_fts fts ON b.rowid = fts.rowid
			WHERE blocks_fts MATCH ?
			ORDER BY b.created_at DESC
			LIMIT ?`
		args = []interface{}{escapedQuery, limit}
	} else {
		sqlQuery = `
			SELECT id, text, created_at, source, datasource, metadata, hostname
			FROM blocks
			ORDER BY created_at DESC
			LIMIT ?`
		args = []interface{}{limit}
	}

	rows, err := s.db.Query(sqlQuery, args...)
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

// SearchBlocksByTimeWithDateRange searches blocks within a date range and orders them by creation time (newest first)
func (s *GenericStorage) SearchBlocksByTimeWithDateRange(query string, limit int, startDate, endDate *time.Time) ([]core.Block, error) {
	var sqlQuery string
	var args []interface{}

	// Build the date range conditions
	var dateConditions []string
	if startDate != nil {
		dateConditions = append(dateConditions, "b.created_at >= ?")
		args = append(args, startDate.Format(time.RFC3339))
	}
	if endDate != nil {
		dateConditions = append(dateConditions, "b.created_at <= ?")
		args = append(args, endDate.Format(time.RFC3339))
	}

	var whereClause string
	if len(dateConditions) > 0 {
		whereClause = " AND " + strings.Join(dateConditions, " AND ")
	}

	if query != "" {
		// Escape FTS5 query for special characters
		escapedQuery := escapeFTS5Query(query)
		sqlQuery = `
			SELECT b.id, b.text, b.created_at, b.source, b.datasource, b.metadata, b.hostname
			FROM blocks b
			JOIN blocks_fts fts ON b.rowid = fts.rowid
			WHERE blocks_fts MATCH ?` + whereClause + `
			ORDER BY b.created_at DESC
			LIMIT ?`
		args = append([]interface{}{escapedQuery}, args...)
		args = append(args, limit)
	} else {
		if len(dateConditions) > 0 {
			whereClause = " WHERE " + strings.Join(dateConditions, " AND ")
		}
		sqlQuery = `
			SELECT id, text, created_at, source, datasource, metadata, hostname
			FROM blocks` + whereClause + `
			ORDER BY created_at DESC
			LIMIT ?`
		args = append(args, limit)
	}

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("querying blocks with date range: %w", err)
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

func (s *GenericStorage) UpdateLastFetchTime(t time.Time) error {
	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO fetch_metadata (key, value, updated_at)
		VALUES ('last_fetch', ?, ?)
	`, t.Format(time.RFC3339), time.Now())

	return err
}

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

func (s *GenericStorage) ExecuteQuery(query string, args ...interface{}) (*sql.Rows, error) {
	return s.db.Query(query, args...)
}

func (s *GenericStorage) ExecuteStatement(query string, args ...interface{}) (sql.Result, error) {
	return s.db.Exec(query, args...)
}

func (s *GenericStorage) Optimize() error {
	_, err := s.db.Exec("PRAGMA optimize")
	return err
}

func (s *GenericStorage) Analyze() error {
	_, err := s.db.Exec("ANALYZE")
	return err
}

func (s *GenericStorage) Vacuum() error {
	_, err := s.db.Exec("VACUUM")
	return err
}

func (s *GenericStorage) WALCheckpoint() error {
	_, err := s.db.Exec("PRAGMA wal_checkpoint(TRUNCATE)")
	return err
}

// escapeFTS5Query prevents SQL injection while allowing all FTS5 syntax
func escapeFTS5Query(query string) string {
	// The query is used in a parameterized query with MATCH ?,
	// so SQL injection is already prevented by SQLite's parameter binding.
	// We just need to return the query as-is to allow full FTS5 syntax.
	return query
}
