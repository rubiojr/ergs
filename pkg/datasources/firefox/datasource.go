package firefox

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
	core.RegisterDatasourcePrototype("firefox", prototype)
}

type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	url := getStringFromMetadata(metadata, "url", "")
	title := getStringFromMetadata(metadata, "title", "")
	description := getStringFromMetadata(metadata, "description", "")

	visitDate := createdAt

	return NewVisitBlockWithSource(id, url, title, description, visitDate, source)
}

type Config struct {
	DatabasePath string `toml:"database_path"`
}

func (c *Config) Validate() error {
	if c.DatabasePath == "" {
		return fmt.Errorf("database_path is required")
	}

	if _, err := os.Stat(c.DatabasePath); os.IsNotExist(err) {
		return fmt.Errorf("database file does not exist: %s", c.DatabasePath)
	}

	return nil
}

type Datasource struct {
	config       *Config
	instanceName string
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var ffConfig *Config
	if config == nil {
		ffConfig = &Config{}
	} else {
		var ok bool
		ffConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for Firefox datasource")
		}
	}

	return &Datasource{
		config:       ffConfig,
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "firefox"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"url":         "TEXT",
		"title":       "TEXT",
		"description": "TEXT",
		"visit_date":  "TEXT",
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
	return fmt.Errorf("invalid config type for Firefox datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	log.Printf("Fetching Firefox browsing history from %s", d.config.DatabasePath)

	tempDir, err := os.MkdirTemp("", "firefox_import_*")
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
		return fmt.Errorf("required Firefox tables not found in database")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT p.url, p.title, p.description, h.visit_date, h.id
		FROM moz_places p
		INNER JOIN moz_historyvisits h
		ON p.id = h.place_id
		ORDER BY h.visit_date DESC
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
		var title, description sql.NullString
		var visitDate int64
		var visitID int64

		if err := rows.Scan(&url, &title, &description, &visitDate, &visitID); err != nil {
			log.Printf("Failed to scan row: %v", err)
			continue
		}

		timestamp := time.Unix(0, visitDate*1000)
		blockID := fmt.Sprintf("firefox-visit-%d", visitID)

		block := NewVisitBlockWithSource(
			blockID,
			url,
			title.String,
			description.String,
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

	log.Printf("Fetched %d Firefox visits", visitCount)
	return nil
}

func (d *Datasource) checkTables(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='moz_places'").Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking moz_places table existence: %w", err)
	}
	if !exists {
		return false, nil
	}

	err = db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='moz_historyvisits'").Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking moz_historyvisits table existence: %w", err)
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
