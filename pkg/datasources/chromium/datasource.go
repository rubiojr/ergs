package chromium

import (
	"context"
	"database/sql"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("chromium", prototype)
}

type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	url := getStringFromMetadata(metadata, "url", "")
	title := getStringFromMetadata(metadata, "title", "")

	visitDate := createdAt

	return NewVisitBlockWithSource(id, url, title, visitDate, source)
}

type Config struct {
	DatabasePath string `toml:"database_path"`
}

func (c *Config) Validate() error {
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}

	// Check if database exists, but only warn if it doesn't
	// This allows the datasource to be configured even if Chromium isn't installed yet
	if _, err := os.Stat(c.DatabasePath); os.IsNotExist(err) {
		log.Printf("Warning: Chromium database does not exist: %s", c.DatabasePath)
		log.Printf("The datasource will fail during fetch until the database is available")
	}

	return nil
}

type Datasource struct {
	config       *Config
	instanceName string
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var chromiumConfig *Config
	if config == nil {
		chromiumConfig = &Config{}
	} else {
		var ok bool
		chromiumConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for Chromium datasource")
		}
	}

	return &Datasource{
		config:       chromiumConfig,
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "chromium"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"url":        "TEXT",
		"title":      "TEXT",
		"visit_date": "TEXT",
	}
}

func (d *Datasource) BlockPrototype() core.Block {
	return &VisitBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		d.config = cfg
		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for Chromium datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching Chromium browsing history from %s", d.config.DatabasePath)

	// Check if database exists before attempting to fetch
	if _, err := os.Stat(d.config.DatabasePath); os.IsNotExist(err) {
		return fmt.Errorf("database file does not exist: %s (is Chromium installed?)", d.config.DatabasePath)
	}

	tempDir, err := os.MkdirTemp("", "chromium_import_*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			fmt.Printf("Warning: failed to remove temp directory: %v\n", err)
		}
	}()

	db, err := d.openDB(d.config.DatabasePath, tempDir)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			fmt.Printf("Warning: failed to close database: %v\n", err)
		}
	}()

	if err := db.PingContext(ctx); err != nil {
		return fmt.Errorf("verifying database connection: %w", err)
	}

	ok, err := d.checkTables(ctx, db)
	if err != nil {
		return fmt.Errorf("checking required tables: %w", err)
	}
	if !ok {
		return fmt.Errorf("required Chromium tables not found in database")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT u.url, u.title, v.visit_time, v.id
		FROM urls u
		INNER JOIN visits v
		ON u.id = v.url
		ORDER BY v.visit_time DESC
	`)
	if err != nil {
		return fmt.Errorf("querying database: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Warning: failed to close rows: %v\n", err)
		}
	}()

	visitCount := 0
	for rows.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var url string
		var title sql.NullString
		var visitTime int64
		var visitID int64

		if err := rows.Scan(&url, &title, &visitTime, &visitID); err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		// Convert Chrome/WebKit timestamp (microseconds since 1601-01-01) to Unix time
		// Chrome epoch is 11644473600 seconds before Unix epoch
		timestamp := chromeTimeToUnix(visitTime)
		blockID := fmt.Sprintf("chromium-visit-%d", visitID)

		block := NewVisitBlockWithSource(
			blockID,
			url,
			title.String,
			timestamp,
			d.instanceName,
		)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
			visitCount++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("row iteration error: %w", err)
	}

	log.Printf("Fetched %d Chromium visits", visitCount)
	return nil
}

func (d *Datasource) checkTables(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='urls'").Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking urls table existence: %w", err)
	}
	if !exists {
		return false, nil
	}

	err = db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='visits'").Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking visits table existence: %w", err)
	}

	return exists, nil
}

func (d *Datasource) openDB(src, tempDir string) (*sql.DB, error) {
	tmpDB := filepath.Join(tempDir, filepath.Base(src))

	sourceFile, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("opening source file: %w", err)
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			fmt.Printf("Warning: failed to close source file: %v\n", err)
		}
	}()

	destFile, err := os.Create(tmpDB)
	if err != nil {
		return nil, fmt.Errorf("creating destination file: %w", err)
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			fmt.Printf("Warning: failed to close destination file: %v\n", err)
		}
	}()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return nil, fmt.Errorf("copying file contents from %s to %s: %w", src, tmpDB, err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", tmpDB))
	if err != nil {
		return nil, fmt.Errorf("opening database in read-only mode: %w", err)
	}

	return db, nil
}

func (d *Datasource) Close() error {
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}

// chromeTimeToUnix converts Chrome/WebKit timestamp (microseconds since 1601-01-01)
// to Unix time (time.Time)
func chromeTimeToUnix(chromeTime int64) time.Time {
	// Chrome epoch is 11644473600 seconds before Unix epoch
	const chromeEpochOffset = 11644473600

	// Convert microseconds to seconds and subtract the offset
	unixSeconds := (chromeTime / 1000000) - chromeEpochOffset

	// Get remaining microseconds for nanosecond precision
	nanos := (chromeTime % 1000000) * 1000

	return time.Unix(unixSeconds, nanos)
}
