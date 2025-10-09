package db

import (
	"database/sql"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

//go:embed migrations/*.sql
var migrationsFS embed.FS

// Migration represents a database migration
type Migration struct {
	Version   int
	Name      string
	SQL       string
	AppliedAt *time.Time
}

// MigrationManager handles database migrations
type MigrationManager struct {
	db             *sql.DB
	migrationsPath string // If set, load from filesystem instead of embedded
}

// NewMigrationManager creates a new migration manager using embedded migrations
func NewMigrationManager(db *sql.DB) *MigrationManager {
	return &MigrationManager{db: db}
}

// NewMigrationManagerFromPath creates a migration manager that loads migrations from a directory
// This is used for testing to allow custom migration scenarios
func NewMigrationManagerFromPath(db *sql.DB, migrationsPath string) *MigrationManager {
	return &MigrationManager{
		db:             db,
		migrationsPath: migrationsPath,
	}
}

// EnsureMigrationsTable creates the migrations table if it doesn't exist
func (m *MigrationManager) EnsureMigrationsTable() error {
	query := `
		CREATE TABLE IF NOT EXISTS migrations (
			version INTEGER PRIMARY KEY,
			applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)
	`
	_, err := m.db.Exec(query)
	return err
}

// GetAppliedMigrations returns a list of applied migration versions
func (m *MigrationManager) GetAppliedMigrations() (map[int]time.Time, error) {
	applied := make(map[int]time.Time)

	rows, err := m.db.Query("SELECT version, applied_at FROM migrations ORDER BY version")
	if err != nil {
		return nil, fmt.Errorf("querying applied migrations: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			// Log error but don't fail the operation
			fmt.Printf("Warning: failed to close rows: %v\n", err)
		}
	}()

	for rows.Next() {
		var version int
		var appliedAt time.Time
		if err := rows.Scan(&version, &appliedAt); err != nil {
			return nil, fmt.Errorf("scanning migration row: %w", err)
		}
		applied[version] = appliedAt
	}

	return applied, rows.Err()
}

// GetAvailableMigrations returns all available migrations from embedded filesystem or directory
func (m *MigrationManager) GetAvailableMigrations() ([]Migration, error) {
	if m.migrationsPath != "" {
		return m.getAvailableMigrationsFromPath()
	}
	return m.getAvailableMigrationsFromEmbed()
}

// getAvailableMigrationsFromEmbed reads migrations from embedded filesystem
func (m *MigrationManager) getAvailableMigrationsFromEmbed() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Extract version from filename (e.g., "001_initial.sql" -> 1)
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		// Read migration content
		content, err := migrationsFS.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading migration file %s: %w", entry.Name(), err)
		}

		// Extract name from filename
		name := strings.TrimSuffix(parts[1], ".sql")

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// getAvailableMigrationsFromPath reads migrations from filesystem directory
func (m *MigrationManager) getAvailableMigrationsFromPath() ([]Migration, error) {
	entries, err := os.ReadDir(m.migrationsPath)
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory %s: %w", m.migrationsPath, err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		// Extract version from filename (e.g., "001_initial.sql" -> 1)
		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		// Read migration content
		content, err := os.ReadFile(filepath.Join(m.migrationsPath, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading migration file %s: %w", entry.Name(), err)
		}

		// Extract name from filename
		name := strings.TrimSuffix(parts[1], ".sql")

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	// Sort by version
	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}

// GetPendingMigrations returns migrations that haven't been applied yet
func (m *MigrationManager) GetPendingMigrations() ([]Migration, error) {
	applied, err := m.GetAppliedMigrations()
	if err != nil {
		return nil, err
	}

	available, err := m.GetAvailableMigrations()
	if err != nil {
		return nil, err
	}

	var pending []Migration
	for _, migration := range available {
		if _, exists := applied[migration.Version]; !exists {
			pending = append(pending, migration)
		}
	}

	return pending, nil
}

// ApplyMigration applies a single migration
func (m *MigrationManager) ApplyMigration(migration Migration) error {

	tx, err := m.db.Begin()
	if err != nil {
		return fmt.Errorf("beginning transaction: %w", err)
	}

	committed := false
	defer func() {
		if !committed {
			if err := tx.Rollback(); err != nil {
				fmt.Printf("Warning: failed to rollback migration transaction: %v\n", err)
			}
		}
	}()

	// Execute the migration SQL
	if _, err := tx.Exec(migration.SQL); err != nil {
		return fmt.Errorf("executing migration %d: %w", migration.Version, err)
	}

	// Record that this migration was applied
	if _, err := tx.Exec("INSERT INTO migrations (version) VALUES (?)", migration.Version); err != nil {
		return fmt.Errorf("recording migration %d: %w", migration.Version, err)
	}

	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing migration %d: %w", migration.Version, err)
	}

	committed = true
	return nil
}

// ApplyPendingMigrations applies all pending migrations
func (m *MigrationManager) ApplyPendingMigrations() error {
	if err := m.EnsureMigrationsTable(); err != nil {
		return fmt.Errorf("ensuring migrations table: %w", err)
	}

	pending, err := m.GetPendingMigrations()
	if err != nil {
		return fmt.Errorf("getting pending migrations: %w", err)
	}

	if len(pending) == 0 {
		return nil
	}

	for _, migration := range pending {
		fmt.Printf("Applying migration %d: %s\n", migration.Version, migration.Name)
		if err := m.ApplyMigration(migration); err != nil {
			return fmt.Errorf("applying migration %d (%s): %w", migration.Version, migration.Name, err)
		}
	}

	fmt.Printf("Applied %d migrations\n", len(pending))
	return nil
}

// GetMigrationStatus returns the current migration status
func (m *MigrationManager) GetMigrationStatus() (*MigrationStatus, error) {
	if err := m.EnsureMigrationsTable(); err != nil {
		return nil, fmt.Errorf("ensuring migrations table: %w", err)
	}

	applied, err := m.GetAppliedMigrations()
	if err != nil {
		return nil, err
	}

	available, err := m.GetAvailableMigrations()
	if err != nil {
		return nil, err
	}

	pending, err := m.GetPendingMigrations()
	if err != nil {
		return nil, err
	}

	status := &MigrationStatus{
		Applied:   make([]Migration, 0, len(applied)),
		Pending:   pending,
		Available: available,
	}

	// Build applied migrations list with timestamps
	for _, migration := range available {
		if appliedAt, exists := applied[migration.Version]; exists {
			migration.AppliedAt = &appliedAt
			status.Applied = append(status.Applied, migration)
		}
	}

	return status, nil
}

// MigrationStatus represents the current state of migrations
type MigrationStatus struct {
	Applied   []Migration
	Pending   []Migration
	Available []Migration
}

// InitializeDatabase initializes a new database with the current schema
func InitializeDatabase(db *sql.DB) error {
	manager := NewMigrationManager(db)

	// Apply all available migrations
	if err := manager.ApplyPendingMigrations(); err != nil {
		return fmt.Errorf("applying migrations: %w", err)
	}

	return nil
}

// InitializeDatabaseFromPath initializes a new database using migrations from a directory
// This is used for testing with custom migration scenarios
func InitializeDatabaseFromPath(db *sql.DB, migrationsPath string) error {
	manager := NewMigrationManagerFromPath(db, migrationsPath)

	// Apply all available migrations
	if err := manager.ApplyPendingMigrations(); err != nil {
		return fmt.Errorf("applying migrations from %s: %w", migrationsPath, err)
	}

	return nil
}

// GetEmbeddedMigrations returns all embedded migration definitions (version, name, SQL)
// without needing a database handle. This allows test helpers or other tooling
// to reuse the canonical embedded migration set instead of duplicating SQL.
func GetEmbeddedMigrations() ([]Migration, error) {
	entries, err := migrationsFS.ReadDir("migrations")
	if err != nil {
		return nil, fmt.Errorf("reading migrations directory: %w", err)
	}

	var migrations []Migration
	for _, entry := range entries {
		if !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		parts := strings.SplitN(entry.Name(), "_", 2)
		if len(parts) != 2 {
			continue
		}

		version, err := strconv.Atoi(parts[0])
		if err != nil {
			continue
		}

		content, err := migrationsFS.ReadFile(filepath.Join("migrations", entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading migration file %s: %w", entry.Name(), err)
		}

		name := strings.TrimSuffix(parts[1], ".sql")

		migrations = append(migrations, Migration{
			Version: version,
			Name:    name,
			SQL:     string(content),
		})
	}

	sort.Slice(migrations, func(i, j int) bool {
		return migrations[i].Version < migrations[j].Version
	})

	return migrations, nil
}
