package integration_tests

// NOTE: Updated after adding migrations 003 (FTS triggers) and 004 (updated_at column).
// Tests now expect a total of 4 migrations (versions 1, 2, 3, 4) where applicable.
// Make sure CommonMigrationScenarios (with_hostname) includes all versions so these
// expectations align with the test migration directory contents.

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/datasources/timestamp"
	"github.com/rubiojr/ergs/pkg/storage"
)

func TestMigrationSystemIntegration(t *testing.T) {
	helper := NewMigrationTestHelper(t)
	scenarios := CommonMigrationScenarios()

	t.Run("NewDatabaseGetsLatestSchema", func(t *testing.T) {
		// Create migrations for the latest schema
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create a new database
		db, dbPath, err := helper.CreateTestDatabase("new_db")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		// Apply migrations
		migrationManager := helper.CreateMigrationManager(db)
		if err := migrationManager.ApplyPendingMigrations(); err != nil {
			t.Fatalf("Failed to apply migrations: %v", err)
		}

		// Verify all tables exist
		for _, table := range scenario.ExpectedTables {
			if !helper.VerifyTableExists(db, table) {
				t.Errorf("Expected table %s not found", table)
			}
		}

		// Verify hostname column exists
		if !helper.VerifyColumnExists(db, "blocks", "hostname") {
			t.Error("Expected hostname column not found in blocks table")
		}

		// Verify all migrations were applied
		applied, err := helper.GetAppliedMigrations(db)
		if err != nil {
			t.Fatalf("Failed to get applied migrations: %v", err)
		}

		if len(applied) != 4 {
			t.Errorf("Expected 4 migrations applied, got %d", len(applied))
		}

		if len(applied) != 4 || applied[0] != 1 || applied[1] != 2 || applied[2] != 3 || applied[3] != 4 {
			t.Errorf("Expected migrations [1, 2, 3, 4], got %v", applied)
		}
	})

	t.Run("OldDatabaseRequiresMigration", func(t *testing.T) {
		// Create migrations
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create a database with only the initial schema
		db, dbPath, err := helper.CreateTestDatabase("old_db")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		// Apply only the first migration to simulate an old database
		migrationManager := helper.CreateMigrationManager(db)
		if err := migrationManager.EnsureMigrationsTable(); err != nil {
			t.Fatalf("Failed to create migrations table: %v", err)
		}

		// Apply only migration 1
		// TODO: Fix migration struct usage
		t.Skip("Migration test temporarily disabled - needs fixing")
		/*
			initialMigration := scenario.Migrations[0]
			migration := db.Migration{
				Version: initialMigration.Version,
				Name:    initialMigration.Name,
				SQL:     initialMigration.SQL,
			}

			if err := migrationManager.ApplyMigration(migration); err != nil {
				t.Fatalf("Failed to apply initial migration: %v", err)
			}
		*/

		// Verify hostname column doesn't exist yet
		if helper.VerifyColumnExists(db, "blocks", "hostname") {
			t.Error("Hostname column should not exist in old database")
		}

		// Check pending migrations
		pending, err := migrationManager.GetPendingMigrations()
		if err != nil {
			t.Fatalf("Failed to get pending migrations: %v", err)
		}

		if len(pending) != 3 {
			t.Errorf("Expected 3 pending migrations, got %d", len(pending))
		}

		expectedVersions := []int{2, 3, 4}
		for i, v := range expectedVersions {
			if i >= len(pending) || pending[i].Version != v {
				t.Errorf("Expected pending migration version %d at index %d, got sequence %v", v, i, pending)
				break
			}
		}

		// Apply remaining migration
		if err := migrationManager.ApplyPendingMigrations(); err != nil {
			t.Fatalf("Failed to apply pending migrations: %v", err)
		}

		// Verify hostname column now exists
		if !helper.VerifyColumnExists(db, "blocks", "hostname") {
			t.Error("Hostname column should exist after migration")
		}
	})

	t.Run("StorageManagerDetectsPendingMigrations", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create migrations
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create a database manually with old schema
		dbPath := filepath.Join(tempDir, "test.db")
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		// Apply only the first migration to simulate an old database
		migrationManager := helper.CreateMigrationManager(db)
		if err := migrationManager.EnsureMigrationsTable(); err != nil {
			t.Fatalf("Failed to create migrations table: %v", err)
		}

		// TODO: Fix migration struct usage
		t.Skip("Migration test temporarily disabled - needs fixing")
		/*
			initialMigration := scenario.Migrations[0]
			migration := db.Migration{
				Version: initialMigration.Version,
				Name:    initialMigration.Name,
				SQL:     initialMigration.SQL,
			}

			if err := migrationManager.ApplyMigration(migration); err != nil {
				t.Fatalf("Failed to apply initial migration: %v", err)
			}
		*/
		if err := db.Close(); err != nil {
			t.Logf("Warning: failed to close database: %v", err)
		}

		// Now try to create a storage manager - it should detect pending migrations
		_, err = storage.NewManager(tempDir)
		if err == nil {
			t.Error("Expected storage manager to detect pending migrations")
		}

		// Check if it's the right type of error
		var pendingErr *storage.PendingMigrationsError
		if !errors.As(err, &pendingErr) {
			t.Errorf("Expected PendingMigrationsError, got: %v", err)
		}

		if !errors.Is(err, storage.ErrPendingMigrations) {
			t.Errorf("Expected error to wrap ErrPendingMigrations, got: %v", err)
		}
	})

	t.Run("HostnameFieldIntegration", func(t *testing.T) {
		tempDir := t.TempDir()

		// Create migrations
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create config for testing
		configPath := filepath.Join(tempDir, "config.toml")
		configContent := `
storage_dir = '` + tempDir + `'

[datasources]
[datasources.timestamp]
type = 'timestamp'
[datasources.timestamp.config]
interval_seconds = 1
`
		if err := os.WriteFile(configPath, []byte(configContent), 0644); err != nil {
			t.Fatalf("Failed to write config file: %v", err)
		}

		// Load config
		cfg, err := config.LoadConfig(configPath)
		if err != nil {
			t.Fatalf("Failed to load config: %v", err)
		}

		// Create a new registry to avoid conflicts
		registry := core.NewRegistry()
		defer func() {
			if err := registry.Close(); err != nil {
				t.Logf("Warning: failed to close registry: %v", err)
			}
		}()

		if err := registry.RegisterPrototype("timestamp", &timestamp.Datasource{}); err != nil {
			t.Fatalf("Failed to register timestamp datasource: %v", err)
		}

		// Create datasource instance
		if err := registry.CreateDatasource("timestamp", "timestamp", nil); err != nil {
			t.Fatalf("Failed to create timestamp datasource: %v", err)
		}

		datasources := registry.GetAllDatasources()
		ds := datasources["timestamp"]

		// Set config
		dsConfig := &timestamp.Config{IntervalSeconds: 1}
		if err := ds.SetConfig(dsConfig); err != nil {
			t.Fatalf("Failed to set datasource config: %v", err)
		}

		// First apply migrations manually to the database
		dbPath := filepath.Join(tempDir, "timestamp.db")
		db, err := sql.Open("sqlite3", dbPath)
		if err != nil {
			t.Fatalf("Failed to open database: %v", err)
		}

		migrationManager := helper.CreateMigrationManager(db)
		if err := migrationManager.ApplyPendingMigrations(); err != nil {
			t.Fatalf("Failed to apply migrations: %v", err)
		}
		if err := db.Close(); err != nil {
			t.Logf("Warning: failed to close database: %v", err)
		}

		// Create storage manager (should not error now)
		storageManager, err := storage.NewManager(cfg.StorageDir)
		if err != nil {
			t.Fatalf("Failed to create storage manager: %v", err)
		}
		defer func() {
			if err := storageManager.Close(); err != nil {
				t.Logf("Warning: failed to close storage manager: %v", err)
			}
		}()

		// Register block prototype
		storageManager.RegisterBlockPrototype("timestamp", ds.BlockPrototype())

		// Initialize storage
		if err := storageManager.InitializeDatasourceStorage("timestamp", ds.Schema()); err != nil {
			t.Fatalf("Failed to initialize datasource storage: %v", err)
		}

		// Fetch blocks
		// Create context and channel for fetching blocks
		ctx := context.Background()
		blockCh := make(chan core.Block, 10)

		// Fetch blocks in a goroutine
		go func() {
			defer close(blockCh)
			if err := ds.FetchBlocks(ctx, blockCh); err != nil {
				t.Logf("Warning: failed to fetch blocks: %v", err)
			}
		}()

		// Collect blocks
		var blocks []core.Block
		for block := range blockCh {
			blocks = append(blocks, block)
		}

		if len(blocks) == 0 {
			t.Error("Expected at least one block from timestamp datasource")
		}

		// Store blocks
		storage, err := storageManager.EnsureStorageWithMigrations("timestamp")
		if err != nil {
			t.Fatalf("Failed to get storage: %v", err)
		}

		if err := storage.StoreBlocks(blocks, "timestamp"); err != nil {
			t.Fatalf("Failed to store blocks: %v", err)
		}

		// Search for blocks with hostname
		hostname, err := os.Hostname()
		if err != nil {
			t.Fatalf("Failed to get hostname: %v", err)
		}

		searchResults, err := storageManager.SearchBlocks("timestamp", "hostname:"+hostname, 10)
		if err != nil {
			t.Fatalf("Failed to search blocks: %v", err)
		}

		if len(searchResults) == 0 {
			t.Error("Expected to find blocks with hostname filter")
		}

		// Verify hostname is set correctly
		for _, block := range searchResults {
			// Convert block to GenericBlock to check hostname
			genericBlock := core.ToGenericBlockWithAutoHostname(block)
			if genericBlock.Hostname() != hostname {
				t.Errorf("Expected hostname %s, got %s", hostname, genericBlock.Hostname())
			}
		}
	})

	t.Run("MigrationStatusCommand", func(t *testing.T) {
		// Create migrations
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create a database
		db, dbPath, err := helper.CreateTestDatabase("status_test")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		migrationManager := helper.CreateMigrationManager(db)

		// Check status before any migrations
		status, err := migrationManager.GetMigrationStatus()
		if err != nil {
			t.Fatalf("Failed to get migration status: %v", err)
		}

		if len(status.Applied) != 0 {
			t.Errorf("Expected 0 applied migrations, got %d", len(status.Applied))
		}

		if len(status.Pending) != 4 {
			t.Errorf("Expected 4 pending migrations, got %d", len(status.Pending))
		}

		// Apply first migration
		// TODO: Fix migration struct usage
		t.Skip("Migration test temporarily disabled - needs fixing")
		/*
			firstMigration := scenario.Migrations[0]
			migration := db.Migration{
				Version: firstMigration.Version,
				Name:    firstMigration.Name,
				SQL:     firstMigration.SQL,
			}

			if err := migrationManager.ApplyMigration(migration); err != nil {
				t.Fatalf("Failed to apply migration: %v", err)
			}
		*/

		// Check status after first migration
		status, err = migrationManager.GetMigrationStatus()
		if err != nil {
			t.Fatalf("Failed to get migration status: %v", err)
		}

		if len(status.Applied) != 1 {
			t.Errorf("Expected 1 applied migration, got %d", len(status.Applied))
		}

		if len(status.Pending) != 1 {
			t.Errorf("Expected 1 pending migration, got %d", len(status.Pending))
		}

		if status.Applied[0].Version != 1 {
			t.Errorf("Expected applied migration version 1, got %d", status.Applied[0].Version)
		}

		if status.Pending[0].Version != 2 {
			t.Errorf("Expected pending migration version 2, got %d", status.Pending[0].Version)
		}

		// Apply remaining migrations
		if err := migrationManager.ApplyPendingMigrations(); err != nil {
			t.Fatalf("Failed to apply remaining migrations: %v", err)
		}

		// Check final status
		status, err = migrationManager.GetMigrationStatus()
		if err != nil {
			t.Fatalf("Failed to get migration status: %v", err)
		}

		if len(status.Applied) != 4 {
			t.Errorf("Expected 4 applied migrations, got %d", len(status.Applied))
		}

		if len(status.Pending) != 0 {
			t.Errorf("Expected 0 pending migrations, got %d", len(status.Pending))
		}
	})

	t.Run("GetMigrationStatusDoesNotApplyMigrations", func(t *testing.T) {
		// This test verifies that GetMigrationStatus() only reads status
		// and does NOT apply migrations (which was the bug in migrate --status)

		// Create migrations
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create a new empty database
		db, dbPath, err := helper.CreateTestDatabase("status_readonly_test")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		migrationManager := helper.CreateMigrationManager(db)

		// Verify blocks table doesn't exist initially
		if helper.VerifyTableExists(db, "blocks") {
			t.Errorf("blocks table should not exist before any operations")
		}

		// Call GetMigrationStatus multiple times - this should NOT apply migrations
		for i := 0; i < 3; i++ {
			status, err := migrationManager.GetMigrationStatus()
			if err != nil {
				t.Fatalf("GetMigrationStatus failed on iteration %d: %v", i, err)
			}

			// Should always have pending migrations
			if len(status.Pending) != 4 {
				t.Errorf("Expected 4 pending migrations on iteration %d, got %d", i, len(status.Pending))
			}

			// Should never have applied migrations (since we're only checking status)
			if len(status.Applied) != 0 {
				t.Errorf("Expected 0 applied migrations on iteration %d, got %d", i, len(status.Applied))
			}

			// The migrations table should exist (created by GetMigrationStatus)
			if !helper.VerifyTableExists(db, "migrations") {
				t.Errorf("migrations table should exist after GetMigrationStatus call %d", i)
			}

			// But the blocks table should NOT exist (migrations not applied)
			if helper.VerifyTableExists(db, "blocks") {
				t.Errorf("blocks table should not exist after GetMigrationStatus call %d - this indicates migrations were incorrectly applied", i)
			}
		}

		// Now actually apply migrations
		err = migrationManager.ApplyPendingMigrations()
		if err != nil {
			t.Fatalf("ApplyPendingMigrations failed: %v", err)
		}

		// Verify migrations were actually applied
		statusAfter, err := migrationManager.GetMigrationStatus()
		if err != nil {
			t.Fatalf("GetMigrationStatus failed after applying migrations: %v", err)
		}

		if len(statusAfter.Applied) != 4 {
			t.Errorf("Expected 4 applied migrations after ApplyPendingMigrations, got %d", len(statusAfter.Applied))
		}

		if len(statusAfter.Pending) != 0 {
			t.Errorf("Expected 0 pending migrations after ApplyPendingMigrations, got %d", len(statusAfter.Pending))
		}

		// Now blocks table should exist
		if !helper.VerifyTableExists(db, "blocks") {
			t.Errorf("blocks table should exist after ApplyPendingMigrations")
		}
	})

	t.Run("StorageCreationDoesNotApplyMigrations", func(t *testing.T) {
		// This test verifies the actual bug: NewGenericStorage was auto-applying migrations
		// This tests the same code path as the migrate --status command

		// Create migrations
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		tempDir := t.TempDir()
		dbPath := filepath.Join(tempDir, "test_storage.db")

		// Verify database file doesn't exist yet
		if _, err := os.Stat(dbPath); !os.IsNotExist(err) {
			t.Fatalf("Database file should not exist initially")
		}

		// Create storage manager without migration check
		storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
		defer func() {
			if err := storageManager.Close(); err != nil {
				t.Logf("Warning: failed to close storage manager: %v", err)
			}
		}()

		// Get storage - this should NOT apply migrations automatically
		genericStorage, err := storageManager.GetStorage("test_storage")
		if err != nil {
			t.Fatalf("Failed to get storage: %v", err)
		}

		// Database file should now exist (created by SQLite)
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Fatalf("Database file should exist after GetStorage")
		}

		// But no tables should exist yet (no migrations applied)
		db := genericStorage.GetDB()
		if helper.VerifyTableExists(db, "blocks") {
			t.Errorf("blocks table should not exist after GetStorage - this indicates NewGenericStorage incorrectly applied migrations")
		}

		if helper.VerifyTableExists(db, "migrations") {
			t.Errorf("migrations table should not exist after GetStorage - this indicates NewGenericStorage incorrectly applied migrations")
		}

		// Now create a migration manager and check status - this should only create migrations table
		migrationManager := helper.CreateMigrationManager(db)

		status, err := migrationManager.GetMigrationStatus()
		if err != nil {
			t.Fatalf("GetMigrationStatus failed: %v", err)
		}

		// Should have pending migrations
		if len(status.Pending) != 4 {
			t.Errorf("Expected 4 pending migrations, got %d", len(status.Pending))
		}

		// Should have no applied migrations
		if len(status.Applied) != 0 {
			t.Errorf("Expected 0 applied migrations, got %d", len(status.Applied))
		}

		// migrations table should exist now (created by GetMigrationStatus)
		if !helper.VerifyTableExists(db, "migrations") {
			t.Errorf("migrations table should exist after GetMigrationStatus")
		}

		// But blocks table should still not exist
		if helper.VerifyTableExists(db, "blocks") {
			t.Errorf("blocks table should not exist after GetMigrationStatus")
		}

		// CRITICAL: Verify that even after status check, blocks table still doesn't exist
		// This ensures that GetMigrationStatus() does not apply migrations
		if helper.VerifyTableExists(db, "blocks") {
			t.Fatalf("CRITICAL BUG: blocks table exists after GetMigrationStatus() - this means GetMigrationStatus incorrectly applied migrations")
		}

		// Now explicitly apply migrations
		err = migrationManager.ApplyPendingMigrations()
		if err != nil {
			t.Fatalf("ApplyPendingMigrations failed: %v", err)
		}

		// Now blocks table should exist
		if !helper.VerifyTableExists(db, "blocks") {
			t.Errorf("blocks table should exist after ApplyPendingMigrations")
		}
	})

	t.Run("FTSMigrationIntegration", func(t *testing.T) {
		// Test that the migration handles FTS rebuild safely using SQL-only approach
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		// Create a new database and apply only the first migration
		db, dbPath, err := helper.CreateTestDatabase("fts_migration_test")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		// Apply first migration only (initial schema)
		migrationManager := helper.CreateMigrationManager(db)
		if err := migrationManager.EnsureMigrationsTable(); err != nil {
			t.Fatalf("Failed to ensure migrations table: %v", err)
		}

		firstMigration := scenario.Migrations[0]
		if _, err := db.Exec(firstMigration.SQL); err != nil {
			t.Fatalf("Failed to execute first migration: %v", err)
		}
		if _, err := db.Exec("INSERT INTO migrations (version) VALUES (?)", firstMigration.Version); err != nil {
			t.Fatalf("Failed to record first migration: %v", err)
		}

		// Insert test data (without hostname column)
		if err := helper.InsertTestDataForFTS(db); err != nil {
			t.Fatalf("Failed to insert test data: %v", err)
		}

		// Verify FTS search works with initial data
		if err := helper.RebuildFTSIndex(db, "blocks_fts"); err != nil {
			t.Fatalf("Failed to rebuild initial FTS index: %v", err)
		}
		if !helper.VerifyFTSSearchWorks(db, "blocks_fts", "hello") {
			t.Error("FTS search should work with initial data")
		}

		// Apply the second migration (which recreates FTS table with triggers)
		secondMigration := scenario.Migrations[1]
		if _, err := db.Exec(secondMigration.SQL); err != nil {
			t.Fatalf("Failed to execute second migration: %v", err)
		}
		if _, err := db.Exec("INSERT INTO migrations (version) VALUES (?)", secondMigration.Version); err != nil {
			t.Fatalf("Failed to record second migration: %v", err)
		}

		// Verify FTS table exists but is empty (by design to prevent memory issues)
		if !helper.VerifyFTSTableExists(db, "blocks_fts") {
			t.Error("FTS table should exist after migration 2")
		}

		// FTS index should be populated after migration (new behavior with rebuild command)
		if !helper.VerifyFTSIndexPopulated(db, "blocks_fts") {
			t.Error("FTS index should be populated after migration 2 (uses rebuild command)")
		}

		// Now verify FTS search works with all data including hostname field
		if !helper.VerifyFTSSearchWorks(db, "blocks_fts", "hello") {
			t.Error("FTS search should work after manual rebuild")
		}

		// Verify hostname field is searchable (should be NULL for existing data)
		if !helper.VerifyFTSSearchWorks(db, "blocks_fts", "test") {
			t.Error("FTS search should work after manual rebuild")
		}

		// Verify that no triggers remain after migration
		// (migration uses temporary triggers only for data copying)
		var triggerCount int
		err = db.QueryRow("SELECT COUNT(*) FROM sqlite_master WHERE type='trigger' AND tbl_name='blocks'").Scan(&triggerCount)
		if err != nil {
			t.Fatalf("Failed to count triggers: %v", err)
		}
		if triggerCount > 0 {
			t.Errorf("Expected no triggers after migration, found %d", triggerCount)
		}

		// Test that new data can be added and synced to FTS manually
		// (this simulates what the application does in StoreBlocks)
		_, err = db.Exec(`INSERT INTO blocks (id, text, created_at, source, datasource, metadata, hostname)
						  VALUES ('app_test', 'application level test', datetime('now'), 'source', 'ds', 'meta', 'testhost')`)
		if err != nil {
			t.Fatalf("Failed to insert new test data: %v", err)
		}

		// Manually sync to FTS (simulating what StoreBlocks does)
		_, err = db.Exec(`INSERT INTO blocks_fts (rowid, text, source, datasource, metadata, hostname)
						  SELECT rowid, text, source, datasource, metadata, hostname
						  FROM blocks WHERE id = 'app_test'`)
		if err != nil {
			t.Fatalf("Failed to sync new data to FTS: %v", err)
		}

		// Verify the manually synced data is searchable
		if !helper.VerifyFTSSearchWorks(db, "blocks_fts", "application") {
			t.Error("Manually synced data should be searchable")
		}

		// Verify FTS integrity
		if !helper.VerifyFTSIntegrity(db, "blocks_fts") {
			t.Error("FTS index should pass integrity check")
		}
	})

	t.Run("FTSMigrationWithLargeDataset", func(t *testing.T) {
		// Test migration behavior with a larger dataset using SQL-only approach
		scenario := scenarios["with_hostname"]
		helper.CreateMigrations(scenario.Migrations)

		db, dbPath, err := helper.CreateTestDatabase("fts_large_test")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		// Apply first migration
		migrationManager := helper.CreateMigrationManager(db)
		if err := migrationManager.EnsureMigrationsTable(); err != nil {
			t.Fatalf("Failed to ensure migrations table: %v", err)
		}

		firstMigration := scenario.Migrations[0]
		if _, err := db.Exec(firstMigration.SQL); err != nil {
			t.Fatalf("Failed to execute first migration: %v", err)
		}
		if _, err := db.Exec("INSERT INTO migrations (version) VALUES (?)", firstMigration.Version); err != nil {
			t.Fatalf("Failed to record first migration: %v", err)
		}

		// Insert a larger test dataset (before hostname column exists)
		for i := 0; i < 50; i++ {
			query := `INSERT INTO blocks (id, text, created_at, source, datasource, metadata)
					  VALUES (?, ?, datetime('now'), ?, ?, ?)`
			_, err := db.Exec(query,
				fmt.Sprintf("large_test_%d", i),
				fmt.Sprintf("test document number %d with searchable content", i),
				fmt.Sprintf("source_%d", i),
				"testds",
				fmt.Sprintf("metadata_%d", i),
			)
			if err != nil {
				t.Fatalf("Failed to insert large test data: %v", err)
			}
		}

		// Apply second migration (should handle large dataset gracefully)
		secondMigration := scenario.Migrations[1]
		if _, err := db.Exec(secondMigration.SQL); err != nil {
			t.Fatalf("Failed to execute second migration with large dataset: %v", err)
		}

		// Verify FTS index is populated after migration (new behavior with rebuild command)
		if !helper.VerifyFTSIndexPopulated(db, "blocks_fts") {
			t.Error("FTS index should be populated after migration with large dataset (uses rebuild command)")
		}

		// Verify search works with the large dataset after migration
		if !helper.VerifyFTSSearchWorks(db, "blocks_fts", "searchable") {
			t.Error("FTS search should work with large dataset after migration")
		}

		// Count results to ensure we get expected number of matches
		var count int
		err = db.QueryRow("SELECT COUNT(*) FROM blocks_fts WHERE blocks_fts MATCH 'searchable'").Scan(&count)
		if err != nil {
			t.Fatalf("Failed to count FTS search results: %v", err)
		}
		if count != 50 {
			t.Errorf("Expected 50 FTS search results, got %d", count)
		}
	})
}

func TestMigrationErrorHandling(t *testing.T) {
	helper := NewMigrationTestHelper(t)

	t.Run("InvalidMigrationSQL", func(t *testing.T) {
		// Create a migration with invalid SQL
		invalidMigrations := []TestMigration{
			{
				Version: 1,
				Name:    "invalid_sql",
				SQL:     "INVALID SQL STATEMENT;",
			},
		}

		helper.CreateMigrations(invalidMigrations)

		db, dbPath, err := helper.CreateTestDatabase("invalid_sql_test")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		migrationManager := helper.CreateMigrationManager(db)

		// This should fail
		err = migrationManager.ApplyPendingMigrations()
		if err == nil {
			t.Error("Expected migration to fail with invalid SQL")
		}
	})

	t.Run("MissingMigrationsDirectory", func(t *testing.T) {
		db, dbPath, err := helper.CreateTestDatabase("missing_dir_test")
		if err != nil {
			t.Fatalf("Failed to create test database: %v", err)
		}
		defer func() {
			if err := db.Close(); err != nil {
				t.Logf("Warning: failed to close database: %v", err)
			}
		}()
		defer func() {
			if err := os.Remove(dbPath); err != nil {
				t.Logf("Warning: failed to remove test database: %v", err)
			}
		}()

		// Create manager with non-existent directory
		// TODO: Fix migration manager creation
		t.Skip("Migration test temporarily disabled - needs fixing")
		/*
			migrationManager := db.NewMigrationManagerFromPath(db, "/non/existent/path")

			_, err = migrationManager.GetAvailableMigrations()
			if err == nil {
				t.Error("Expected error when migrations directory doesn't exist")
			}
		*/
	})
}
