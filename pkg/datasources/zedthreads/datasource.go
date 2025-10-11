package zedthreads

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/rubiojr/ergs/pkg/log"

	"github.com/klauspost/compress/zstd"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("zedthreads", prototype)
}

type BlockFactory struct{}

func (f *BlockFactory) CreateFromGeneric(id, text string, createdAt time.Time, source string, metadata map[string]interface{}) core.Block {
	summary := getStringFromMetadata(metadata, "summary", "")
	modelStr := getStringFromMetadata(metadata, "model", "")
	version := getStringFromMetadata(metadata, "version", "")
	messageCount := getIntFromMetadata(metadata, "message_count", 0)

	// userMessages and assistantMessages are stored in metadata but not used here
	// as we don't have enough information to reconstruct the full message structure

	// Parse model string back to Model struct if possible
	var model *Model
	if modelStr != "" {
		// Simple parsing - just store as model name for now
		model = &Model{Model: modelStr}
	}

	// Create a basic ThreadData structure from metadata
	threadData := &ThreadData{
		Version:  version,
		Summary:  summary,
		Model:    model,
		Messages: make([]Message, messageCount),
	}

	// Extract token usage from metadata
	tokenUsage := make(map[string]interface{})
	for key, value := range metadata {
		if len(key) > 6 && key[:6] == "token_" {
			tokenUsage[key[6:]] = value
		}
	}
	threadData.TokenUsage = tokenUsage

	return &ThreadBlock{
		id:         id,
		text:       text,
		createdAt:  createdAt,
		source:     source,
		metadata:   metadata,
		summary:    summary,
		updatedAt:  createdAt,
		messages:   threadData.Messages,
		model:      model,
		tokenUsage: tokenUsage,
	}
}

type Config struct {
	DatabasePath string `toml:"database_path"`
}

func (c *Config) Validate() error {
	// Set default path if not provided
	if c.DatabasePath == "" {
		homeDir, err := os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("could not determine home directory: %w", err)
		}
		c.DatabasePath = filepath.Join(homeDir, ".local", "share", "zed", "threads", "threads.db")
	}

	if _, err := os.Stat(c.DatabasePath); os.IsNotExist(err) {
		return fmt.Errorf("database file does not exist: %s", c.DatabasePath)
	}

	return nil
}

type Datasource struct {
	config       *Config
	decoder      *zstd.Decoder
	instanceName string
}

func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var zedConfig *Config
	if config == nil {
		zedConfig = &Config{}
	} else {
		var ok bool
		zedConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("invalid config type for Zed threads datasource")
		}
	}

	// Validate config (this will set default path if needed)
	if err := zedConfig.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	// Create zstd decoder
	decoder, err := zstd.NewReader(nil)
	if err != nil {
		return nil, fmt.Errorf("creating zstd decoder: %w", err)
	}

	return &Datasource{
		config:       zedConfig,
		decoder:      decoder,
		instanceName: instanceName,
	}, nil
}

func (d *Datasource) Type() string {
	return "zedthreads"
}

func (d *Datasource) Name() string {
	return d.instanceName
}

func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"summary":            "TEXT",
		"updated_at":         "TEXT",
		"model":              "TEXT",
		"version":            "TEXT",
		"message_count":      "INTEGER",
		"user_messages":      "INTEGER",
		"assistant_messages": "INTEGER",
		"token_total":        "INTEGER",
		"token_input":        "INTEGER",
		"token_output":       "INTEGER",
	}
}

func (d *Datasource) BlockPrototype() core.Block {
	return &ThreadBlock{}
}

func (d *Datasource) ConfigType() interface{} {
	return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
	if cfg, ok := config.(*Config); ok {
		d.config = cfg
		return cfg.Validate()
	}
	return fmt.Errorf("invalid config type for Zed threads datasource")
}

func (d *Datasource) GetConfig() interface{} {
	return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	l := log.ForService("zedthreads:" + d.instanceName)
	l.Debugf("Fetching Zed threads from %s", d.config.DatabasePath)

	tempDir, err := os.MkdirTemp("", "zedthreads_import_*")
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

	ok, err := d.checkSchema(ctx, db)
	if err != nil {
		return fmt.Errorf("checking database schema: %w", err)
	}
	if !ok {
		return fmt.Errorf("required Zed threads schema not found in database")
	}

	rows, err := db.QueryContext(ctx, `
		SELECT id, summary, updated_at, data_type, data
		FROM threads
		ORDER BY updated_at DESC
	`)
	if err != nil {
		return fmt.Errorf("querying database: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Warning: failed to close rows: %v\n", err)
		}
	}()

	threadCount := 0
	for rows.Next() {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		var id, summary, updatedAtStr, dataType string
		var data []byte

		if err := rows.Scan(&id, &summary, &updatedAtStr, &dataType, &data); err != nil {
			l.Warnf("Failed to scan row: %v", err)
			continue
		}

		updatedAt, err := time.Parse(time.RFC3339, updatedAtStr)
		if err != nil {
			l.Warnf("Failed to parse timestamp %s: %v", updatedAtStr, err)
			updatedAt = time.Now().UTC()
		} else {
			updatedAt = updatedAt.UTC()
		}

		var threadData *ThreadData
		if dataType == "zstd" {
			threadData, err = d.decompressThreadData(data)
			if err != nil {
				l.Warnf("Failed to decompress thread data for %s: %v", id, err)
				continue
			}
		} else {
			l.Warnf("Unsupported data type %s for thread %s", dataType, id)
			continue
		}

		block := NewThreadBlockWithSource(id, summary, updatedAt, threadData, d.instanceName)

		select {
		case <-ctx.Done():
			return ctx.Err()
		case blockCh <- block:
			threadCount++
		}
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("row iteration error: %w", err)
	}

	l.Debugf("Fetched %d Zed threads", threadCount)
	return nil
}

func (d *Datasource) decompressThreadData(compressedData []byte) (*ThreadData, error) {
	decompressed, err := d.decoder.DecodeAll(compressedData, nil)
	if err != nil {
		return nil, fmt.Errorf("decompressing data: %w", err)
	}

	var threadData ThreadData
	if err := json.Unmarshal(decompressed, &threadData); err != nil {
		return nil, fmt.Errorf("unmarshaling JSON: %w", err)
	}

	return &threadData, nil
}

func (d *Datasource) checkSchema(ctx context.Context, db *sql.DB) (bool, error) {
	var exists bool
	err := db.QueryRowContext(ctx, `
		SELECT 1 FROM sqlite_master
		WHERE type='table' AND name='threads'
	`).Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking threads table existence: %w", err)
	}
	if !exists {
		return false, nil
	}

	// Verify the table has the expected columns
	rows, err := db.QueryContext(ctx, "PRAGMA table_info(threads)")
	if err != nil {
		return false, fmt.Errorf("getting table info: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Warning: failed to close rows: %v\n", err)
		}
	}()

	expectedColumns := map[string]bool{
		"id":         false,
		"summary":    false,
		"updated_at": false,
		"data_type":  false,
		"data":       false,
	}

	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, pk int
		var defaultValue sql.NullString

		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &pk); err != nil {
			return false, fmt.Errorf("scanning column info: %w", err)
		}

		if _, expected := expectedColumns[name]; expected {
			expectedColumns[name] = true
		}
	}

	for column, found := range expectedColumns {
		if !found {
			return false, fmt.Errorf("missing required column: %s", column)
		}
	}

	return true, nil
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
	if d.decoder != nil {
		d.decoder.Close()
	}
	return nil
}

func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}
