package integration_tests

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/rubiojr/ergs/pkg/db"
	"github.com/rubiojr/ergs/pkg/storage"
)

// TestMigrationScenario represents a migration test scenario
type TestMigrationScenario struct {
	Name            string
	Migrations      []TestMigration
	ExpectedTables  []string
	ExpectedColumns map[string][]string // table -> columns
}

// TestMigration represents a single migration for testing
type TestMigration struct {
	Version int
	Name    string
	SQL     string
}

// MigrationTestHelper helps set up migration test scenarios
type MigrationTestHelper struct {
	tempDir       string
	migrationsDir string
	t             *testing.T
}

// NewMigrationTestHelper creates a new migration test helper
func NewMigrationTestHelper(t *testing.T) *MigrationTestHelper {
	tempDir := t.TempDir()
	migrationsDir := filepath.Join(tempDir, "migrations")

	if err := os.MkdirAll(migrationsDir, 0755); err != nil {
		t.Fatalf("Failed to create migrations directory: %v", err)
	}

	return &MigrationTestHelper{
		tempDir:       tempDir,
		migrationsDir: migrationsDir,
		t:             t,
	}
}

// CreateMigrations writes migration files to the test directory
func (h *MigrationTestHelper) CreateMigrations(migrations []TestMigration) {
	for _, migration := range migrations {
		filename := fmt.Sprintf("%03d_%s.sql", migration.Version, migration.Name)
		filepath := filepath.Join(h.migrationsDir, filename)

		if err := os.WriteFile(filepath, []byte(migration.SQL), 0644); err != nil {
			h.t.Fatalf("Failed to write migration file %s: %v", filename, err)
		}
	}
}

// CreateTestDatabase creates a test database with the given name
func (h *MigrationTestHelper) CreateTestDatabase(name string) (*sql.DB, string, error) {
	dbPath := filepath.Join(h.tempDir, fmt.Sprintf("%s.db", name))
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, "", fmt.Errorf("opening database: %w", err)
	}
	return db, dbPath, nil
}

// ApplyMigrations applies migrations from the test directory to a database
func (h *MigrationTestHelper) ApplyMigrations(database *sql.DB) error {
	return db.InitializeDatabaseFromPath(database, h.migrationsDir)
}

// CreateMigrationManager creates a migration manager using the test migrations directory
func (h *MigrationTestHelper) CreateMigrationManager(database *sql.DB) *db.MigrationManager {
	return db.NewMigrationManagerFromPath(database, h.migrationsDir)
}

// CreateStorageManagerWithMigrations creates a storage manager that uses test migrations
func (h *MigrationTestHelper) CreateStorageManagerWithMigrations() *storage.Manager {
	// Create a custom storage manager that uses test migrations
	// This requires modifying the storage.Manager to accept custom migration paths
	return storage.NewManagerWithoutMigrationCheck(h.tempDir)
}

// GetMigrationsDir returns the path to the test migrations directory
func (h *MigrationTestHelper) GetMigrationsDir() string {
	return h.migrationsDir
}

// GetTempDir returns the temporary directory for this test
func (h *MigrationTestHelper) GetTempDir() string {
	return h.tempDir
}

// VerifyTableExists checks if a table exists in the database
func (h *MigrationTestHelper) VerifyTableExists(database *sql.DB, tableName string) bool {
	var count int
	query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?"
	err := database.QueryRow(query, tableName).Scan(&count)
	if err != nil {
		h.t.Logf("Error checking table %s: %v", tableName, err)
		return false
	}
	return count > 0
}

// VerifyColumnExists checks if a column exists in a table
func (h *MigrationTestHelper) VerifyColumnExists(database *sql.DB, tableName, columnName string) bool {
	query := fmt.Sprintf("PRAGMA table_info(%s)", tableName)
	rows, err := database.Query(query)
	if err != nil {
		h.t.Logf("Error getting table info for %s: %v", tableName, err)
		return false
	}
	defer func() {
		if err := rows.Close(); err != nil {
			h.t.Logf("Warning: failed to close rows: %v", err)
		}
	}()

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			h.t.Logf("Error scanning table info: %v", err)
			continue
		}

		if name == columnName {
			return true
		}
	}
	return false
}

// GetAppliedMigrations returns the list of applied migration versions
func (h *MigrationTestHelper) GetAppliedMigrations(database *sql.DB) ([]int, error) {
	query := "SELECT version FROM migrations ORDER BY version"
	rows, err := database.Query(query)
	if err != nil {
		return nil, err
	}
	defer func() {
		if err := rows.Close(); err != nil {
			h.t.Logf("Warning: failed to close rows: %v", err)
		}
	}()

	var versions []int
	for rows.Next() {
		var version int
		if err := rows.Scan(&version); err != nil {
			return nil, err
		}
		versions = append(versions, version)
	}
	return versions, rows.Err()
}

// VerifyFTSTableExists checks if the FTS table exists and is properly configured
func (h *MigrationTestHelper) VerifyFTSTableExists(database *sql.DB, ftsTableName string) bool {
	var count int
	query := "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=? AND sql LIKE '%fts5%'"
	err := database.QueryRow(query, ftsTableName).Scan(&count)
	if err != nil {
		h.t.Logf("Error checking FTS table %s: %v", ftsTableName, err)
		return false
	}
	return count > 0
}

// VerifyFTSIndexPopulated checks if the FTS index contains data
func (h *MigrationTestHelper) VerifyFTSIndexPopulated(database *sql.DB, ftsTableName string) bool {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s", ftsTableName)
	err := database.QueryRow(query).Scan(&count)
	if err != nil {
		h.t.Logf("Error checking FTS index population for %s: %v", ftsTableName, err)
		return false
	}
	return count > 0
}

// VerifyFTSSearchWorks tests that FTS search functionality works
func (h *MigrationTestHelper) VerifyFTSSearchWorks(database *sql.DB, ftsTableName, searchTerm string) bool {
	var count int
	query := fmt.Sprintf("SELECT COUNT(*) FROM %s WHERE %s MATCH ?", ftsTableName, ftsTableName)
	err := database.QueryRow(query, searchTerm).Scan(&count)
	if err != nil {
		h.t.Logf("Error testing FTS search for %s with term '%s': %v", ftsTableName, searchTerm, err)
		return false
	}
	return count >= 0 // Any count >= 0 means the search worked (even if no results)
}

// InsertTestDataForFTS inserts test data that can be used to verify FTS functionality
func (h *MigrationTestHelper) InsertTestDataForFTS(database *sql.DB) error {
	// Check if hostname column exists
	hasHostname := h.VerifyColumnExists(database, "blocks", "hostname")

	testData := []struct {
		id         string
		text       string
		source     string
		datasource string
		metadata   string
		hostname   string
	}{
		{"test1", "hello world testing", "source1", "testds", "meta1", "host1"},
		{"test2", "another test document", "source2", "testds", "meta2", "host2"},
		{"test3", "search functionality verification", "source3", "testds", "meta3", "host1"},
	}

	for _, data := range testData {
		var query string
		var args []interface{}

		if hasHostname {
			query = `INSERT INTO blocks (id, text, created_at, source, datasource, metadata, hostname)
					  VALUES (?, ?, datetime('now'), ?, ?, ?, ?)`
			args = []interface{}{data.id, data.text, data.source, data.datasource, data.metadata, data.hostname}
		} else {
			query = `INSERT INTO blocks (id, text, created_at, source, datasource, metadata)
					  VALUES (?, ?, datetime('now'), ?, ?, ?)`
			args = []interface{}{data.id, data.text, data.source, data.datasource, data.metadata}
		}

		_, err := database.Exec(query, args...)
		if err != nil {
			return fmt.Errorf("inserting test data: %w", err)
		}
	}
	return nil
}

// RebuildFTSIndex rebuilds the FTS index - used for testing FTS rebuild functionality
func (h *MigrationTestHelper) RebuildFTSIndex(database *sql.DB, ftsTableName string) error {
	query := fmt.Sprintf("INSERT INTO %s(%s) VALUES('rebuild')", ftsTableName, ftsTableName)
	_, err := database.Exec(query)
	if err != nil {
		return fmt.Errorf("rebuilding FTS index: %w", err)
	}
	return nil
}

// VerifyFTSIntegrity checks the integrity of the FTS index
func (h *MigrationTestHelper) VerifyFTSIntegrity(database *sql.DB, ftsTableName string) bool {
	query := fmt.Sprintf("INSERT INTO %s(%s) VALUES('integrity-check')", ftsTableName, ftsTableName)
	_, err := database.Exec(query)
	if err != nil {
		h.t.Logf("FTS integrity check failed for %s: %v", ftsTableName, err)
		return false
	}
	return true
}

// CommonMigrationScenarios returns common test scenarios
func CommonMigrationScenarios() map[string]TestMigrationScenario {
	return map[string]TestMigrationScenario{
		"initial_only": {
			Name: "Initial schema only",
			Migrations: []TestMigration{
				{
					Version: 1,
					Name:    "initial_schema",
					SQL: `-- Initial schema
CREATE TABLE IF NOT EXISTS blocks (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    source TEXT NOT NULL,
    datasource TEXT NOT NULL,
    metadata TEXT
);

CREATE TABLE IF NOT EXISTS fetch_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE IF NOT EXISTS blocks_fts USING fts5(
    text,
    source,
    datasource,
    metadata,
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);

CREATE TABLE IF NOT EXISTS migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`,
				},
			},
			ExpectedTables: []string{"blocks", "fetch_metadata", "blocks_fts", "migrations"},
			ExpectedColumns: map[string][]string{
				"blocks": {"id", "text", "created_at", "source", "datasource", "metadata"},
			},
		},
		"with_hostname": {
			Name: "Initial schema plus hostname",
			Migrations: []TestMigration{
				{
					Version: 1,
					Name:    "initial_schema",
					SQL: `-- Initial schema
CREATE TABLE IF NOT EXISTS blocks (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    source TEXT NOT NULL,
    datasource TEXT NOT NULL,
    metadata TEXT
);

CREATE TABLE IF NOT EXISTS fetch_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

CREATE VIRTUAL TABLE IF NOT EXISTS blocks_fts USING fts5(
    text,
    source,
    datasource,
    metadata,
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);

CREATE TABLE IF NOT EXISTS migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);`,
				},
				{
					Version: 2,
					Name:    "add_hostname",
					SQL: `-- Add hostname field to blocks table
ALTER TABLE blocks ADD COLUMN hostname TEXT;

-- Update the FTS index to include hostname
DROP TABLE IF EXISTS blocks_fts;
CREATE VIRTUAL TABLE blocks_fts USING fts5(
    text,
    source,
    datasource,
    metadata,
    hostname,
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);

-- Rebuild FTS index with existing data
INSERT INTO blocks_fts(rowid, text, source, datasource, metadata, hostname)
SELECT rowid, text, source, datasource, metadata, hostname FROM blocks;`,
				},
			},
			ExpectedTables: []string{"blocks", "fetch_metadata", "blocks_fts", "migrations"},
			ExpectedColumns: map[string][]string{
				"blocks": {"id", "text", "created_at", "source", "datasource", "metadata", "hostname"},
			},
		},
	}
}
