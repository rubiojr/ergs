package main

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"time"

	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/core"
)

// ImportRequest is the payload sent to the importer API
type ImportRequest struct {
	Blocks []core.GenericBlock `json:"blocks"`
}

// ImportResponse is the response from the importer API
type ImportResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

type Config struct {
	DatabasePath  string
	ImporterURL   string
	APIKey        string
	TargetDatasrc string
	BatchSize     int
	DryRun        bool
	Limit         int
}

func main() {
	cfg := Config{}

	flag.StringVar(&cfg.DatabasePath, "database-path", "", "Path to Chromium History database (required)")
	flag.StringVar(&cfg.ImporterURL, "importer-url", "http://localhost:9090", "URL of the importer API server")
	flag.StringVar(&cfg.APIKey, "api-key", "", "API token for authentication (required unless --dry-run)")
	flag.StringVar(&cfg.TargetDatasrc, "target-datasource", "chromium", "Target datasource name in Ergs")
	flag.IntVar(&cfg.BatchSize, "batch-size", 100, "Number of blocks to send per request")
	flag.IntVar(&cfg.Limit, "limit", 0, "Maximum number of visits to import (0 = all)")
	flag.BoolVar(&cfg.DryRun, "dry-run", false, "Don't actually send blocks, just show what would be imported")
	flag.Parse()

	if cfg.DatabasePath == "" {
		log.Fatal("Error: --database-path is required")
	}

	if cfg.APIKey == "" && !cfg.DryRun {
		log.Fatal("Error: --api-key is required (unless using --dry-run)")
	}

	if err := run(cfg); err != nil {
		log.Fatalf("Error: %v", err)
	}
}

func run(cfg Config) error {
	log.Printf("Chromium Importer")
	log.Printf("================")
	log.Printf("Database path: %s", cfg.DatabasePath)
	log.Printf("Importer URL: %s", cfg.ImporterURL)
	log.Printf("Target datasource: %s", cfg.TargetDatasrc)
	log.Printf("Batch size: %d", cfg.BatchSize)
	if cfg.Limit > 0 {
		log.Printf("Limit: %d visits", cfg.Limit)
	}
	if cfg.DryRun {
		log.Printf("Mode: DRY RUN (no blocks will be sent)")
	}
	log.Printf("")

	// Verify database exists
	if _, err := os.Stat(cfg.DatabasePath); os.IsNotExist(err) {
		return fmt.Errorf("database file does not exist: %s", cfg.DatabasePath)
	}

	// Create temp copy of database to avoid locking issues
	tempDir, err := os.MkdirTemp("", "chromium_import_*")
	if err != nil {
		return fmt.Errorf("creating temp directory: %w", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			log.Printf("Warning: failed to remove temp directory: %v", err)
		}
	}()

	db, err := openDB(cfg.DatabasePath, tempDir)
	if err != nil {
		return fmt.Errorf("opening database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
	}()

	// Check tables exist
	ok, err := checkTables(db)
	if err != nil {
		return fmt.Errorf("checking tables: %w", err)
	}
	if !ok {
		return fmt.Errorf("required Chromium tables not found in database")
	}

	// Count total visits
	var totalVisits int
	err = db.QueryRow("SELECT COUNT(*) FROM visits").Scan(&totalVisits)
	if err != nil {
		return fmt.Errorf("counting visits: %w", err)
	}

	log.Printf("Found %d visits in database", totalVisits)
	log.Printf("")

	// Query visits
	query := `
		SELECT u.url, u.title, v.visit_time, v.id
		FROM urls u
		INNER JOIN visits v
		ON u.id = v.url
		ORDER BY v.visit_time DESC
	`

	if cfg.Limit > 0 {
		query += fmt.Sprintf(" LIMIT %d", cfg.Limit)
	}

	rows, err := db.Query(query)
	if err != nil {
		return fmt.Errorf("querying database: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Warning: failed to close rows: %v", err)
		}
	}()

	// Process visits in batches
	var batch []core.GenericBlock
	totalProcessed := 0
	totalAccepted := 0
	totalRejected := 0
	var allErrors []string

	for rows.Next() {
		var url string
		var title sql.NullString
		var visitTime int64
		var visitID int64

		if err := rows.Scan(&url, &title, &visitTime, &visitID); err != nil {
			log.Printf("Warning: failed to scan row: %v", err)
			continue
		}

		// Convert Chrome time to Unix time
		timestamp := chromeTimeToUnix(visitTime)
		blockID := fmt.Sprintf("chromium-visit-%d", visitID)

		// Create searchable text
		text := fmt.Sprintf("url=%s title=%s", url, title.String)

		// Create metadata
		metadata := map[string]interface{}{
			"url":        url,
			"title":      title.String,
			"visit_date": timestamp.Format("2006-01-02 15:04:05"),
			"source":     cfg.TargetDatasrc,
		}

		// Create generic block
		block := core.NewGenericBlock(
			blockID,
			text,
			cfg.TargetDatasrc,
			"chromium",
			timestamp,
			metadata,
		)

		batch = append(batch, *block)

		// Send batch when it reaches the configured size
		if len(batch) >= cfg.BatchSize {
			if cfg.DryRun {
				log.Printf("Would import batch of %d blocks (total: %d)", len(batch), totalProcessed+len(batch))
				for i, b := range batch {
					if i < 5 { // Show first 5 of each batch
						log.Printf("  - %s: %s", b.ID(), b.Metadata()["url"])
					}
				}
				if len(batch) > 5 {
					log.Printf("  ... and %d more", len(batch)-5)
				}
			} else {
				resp, err := sendBatch(cfg.ImporterURL, cfg.APIKey, batch)
				if err != nil {
					return fmt.Errorf("sending batch: %w", err)
				}

				totalAccepted += resp.Accepted
				totalRejected += resp.Rejected
				allErrors = append(allErrors, resp.Errors...)

				log.Printf("Batch %d: accepted %d, rejected %d (total: %d)",
					(totalProcessed/cfg.BatchSize)+1, resp.Accepted, resp.Rejected, totalProcessed+len(batch))

				if len(resp.Errors) > 0 {
					for _, errMsg := range resp.Errors {
						log.Printf("  Error: %s", errMsg)
					}
				}
			}

			totalProcessed += len(batch)
			batch = nil
		}
	}

	// Send remaining batch
	if len(batch) > 0 {
		if cfg.DryRun {
			log.Printf("Would import final batch of %d blocks (total: %d)", len(batch), totalProcessed+len(batch))
			for i, b := range batch {
				if i < 5 {
					log.Printf("  - %s: %s", b.ID(), b.Metadata()["url"])
				}
			}
			if len(batch) > 5 {
				log.Printf("  ... and %d more", len(batch)-5)
			}
		} else {
			resp, err := sendBatch(cfg.ImporterURL, cfg.APIKey, batch)
			if err != nil {
				return fmt.Errorf("sending final batch: %w", err)
			}

			totalAccepted += resp.Accepted
			totalRejected += resp.Rejected
			allErrors = append(allErrors, resp.Errors...)

			log.Printf("Final batch: accepted %d, rejected %d (total: %d)",
				resp.Accepted, resp.Rejected, totalProcessed+len(batch))

			if len(resp.Errors) > 0 {
				for _, errMsg := range resp.Errors {
					log.Printf("  Error: %s", errMsg)
				}
			}
		}

		totalProcessed += len(batch)
	}

	if err := rows.Err(); err != nil {
		return fmt.Errorf("row iteration error: %w", err)
	}

	log.Printf("")
	log.Printf("Import complete!")
	log.Printf("Total visits processed: %d", totalProcessed)
	if !cfg.DryRun {
		log.Printf("Total blocks accepted: %d", totalAccepted)
		log.Printf("Total blocks rejected: %d", totalRejected)
		if len(allErrors) > 0 {
			log.Printf("Total errors: %d", len(allErrors))
		}
	}

	return nil
}

func openDB(src, tempDir string) (*sql.DB, error) {
	tmpDB := filepath.Join(tempDir, filepath.Base(src))

	sourceFile, err := os.Open(src)
	if err != nil {
		return nil, fmt.Errorf("opening source file: %w", err)
	}
	defer func() {
		if err := sourceFile.Close(); err != nil {
			log.Printf("Warning: failed to close source file: %v", err)
		}
	}()

	destFile, err := os.Create(tmpDB)
	if err != nil {
		return nil, fmt.Errorf("creating destination file: %w", err)
	}
	defer func() {
		if err := destFile.Close(); err != nil {
			log.Printf("Warning: failed to close destination file: %v", err)
		}
	}()

	if _, err = io.Copy(destFile, sourceFile); err != nil {
		return nil, fmt.Errorf("copying file contents: %w", err)
	}

	db, err := sql.Open("sqlite3", fmt.Sprintf("file:%s?mode=ro", tmpDB))
	if err != nil {
		return nil, fmt.Errorf("opening database in read-only mode: %w", err)
	}

	return db, nil
}

func checkTables(db *sql.DB) (bool, error) {
	ctx := context.Background()

	var exists bool
	err := db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='urls'").Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking urls table: %w", err)
	}
	if !exists {
		return false, nil
	}

	err = db.QueryRowContext(ctx, "SELECT 1 FROM sqlite_master WHERE type='table' AND name='visits'").Scan(&exists)
	if err != nil && err != sql.ErrNoRows {
		return false, fmt.Errorf("checking visits table: %w", err)
	}

	return exists, nil
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

func sendBatch(importerURL, apiKey string, blocks []core.GenericBlock) (*ImportResponse, error) {
	url := fmt.Sprintf("%s/api/import/blocks", importerURL)

	req := ImportRequest{
		Blocks: blocks,
	}

	jsonData, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("marshaling request: %w", err)
	}

	httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(jsonData))
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("sending request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API returned status %d: %s", resp.StatusCode, string(body))
	}

	var importResp ImportResponse
	if err := json.Unmarshal(body, &importResp); err != nil {
		return nil, fmt.Errorf("parsing response: %w", err)
	}

	return &importResp, nil
}
