package cmd

import (
	"context"
	"crypto/rand"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/google/uuid"
	_ "github.com/ncruces/go-sqlite3/driver"
	_ "github.com/ncruces/go-sqlite3/embed"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/urfave/cli/v3"
)

// ImporterCommand creates the importer command
func ImporterCommand() *cli.Command {
	return &cli.Command{
		Name:  "importer",
		Usage: "Start importer API server for receiving blocks from external sources",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "port",
				Usage: "Port to listen on",
				Value: "9090",
			},
			&cli.StringFlag{
				Name:  "host",
				Usage: "Host to bind to",
				Value: "localhost",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return startImporterServer(ctx, c.String("config"), c.String("host"), c.String("port"))
		},
	}
}

type ImporterServer struct {
	db         *sql.DB
	storageDir string
	apiToken   string
}

type ImportBlocksRequest struct {
	Blocks []core.GenericBlock `json:"blocks"`
}

type ImportBlocksResponse struct {
	Accepted int      `json:"accepted"`
	Rejected int      `json:"rejected"`
	Errors   []string `json:"errors,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

func startImporterServer(ctx context.Context, configPath, host, port string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	// Enforce that all datasource databases are fully migrated before
	// starting the importer. This prevents accepting/importing blocks
	// that target schemas which are not yet upgraded.
	if err := CheckPendingMigrations(configPath); err != nil {
		return fmt.Errorf("cannot start importer; %w", err)
	}

	// Use config values if flags are default and config has values
	if cfg.Importer != nil {
		// Use config host if flag is default
		if host == "localhost" && cfg.Importer.Host != "" {
			host = cfg.Importer.Host
		}
		// Use config port if flag is default
		if port == "9090" && cfg.Importer.Port != "" {
			port = cfg.Importer.Port
		}
	}

	// Get or generate API token
	var apiToken string
	if cfg.Importer != nil {
		apiToken = cfg.Importer.APIKey
	}
	if apiToken == "" {
		// Generate a random API token
		apiToken, err = generateAPIToken()
		if err != nil {
			return fmt.Errorf("generating API token: %w", err)
		}
		log.Printf("⚠️  No API key configured. Generated random key for this session.")
		log.Printf("⚠️  Add this to your config.toml to persist it:")
		log.Printf("⚠️  [importer]")
		log.Printf("⚠️  api_key = \"%s\"", apiToken)
	}

	// Ensure internal directory exists
	internalDir := filepath.Join(cfg.StorageDir, "internal")
	if err := os.MkdirAll(internalDir, 0755); err != nil {
		return fmt.Errorf("creating internal directory: %w", err)
	}

	// Initialize database
	dbPath := filepath.Join(internalDir, "importer.db")
	db, err := initImporterDB(dbPath)
	if err != nil {
		return fmt.Errorf("initializing database: %w", err)
	}
	defer func() {
		if err := db.Close(); err != nil {
			log.Printf("Warning: failed to close database: %v", err)
		}
	}()

	server := &ImporterServer{
		db:         db,
		storageDir: cfg.StorageDir,
		apiToken:   apiToken,
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/import/blocks", server.handleImportBlocks)
	mux.HandleFunc("GET /api/blocks/export", server.handleExportBlocks)
	mux.HandleFunc("GET /health", server.handleHealth)
	mux.HandleFunc("GET /api/stats", server.handleStats)

	// Add CORS and auth middleware
	handler := corsMiddleware(server.authMiddleware(mux))

	httpServer := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: handler,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting importer API server on http://%s:%s", host, port)
		log.Printf("")
		log.Printf("🔑 API Key: %s", apiToken)
		log.Printf("")
		log.Printf("Available endpoints:")
		log.Printf("  POST /api/import/blocks - Import blocks (datasource specified in block data)")
		log.Printf("  GET  /api/blocks/export - Export and delete all pending blocks")
		log.Printf("  GET  /health - Health check (no auth required)")
		log.Printf("  GET  /api/stats - Get import statistics")
		log.Printf("")
		log.Printf("Example usage:")
		log.Printf("  curl -X POST http://%s:%s/api/import/blocks \\", host, port)
		log.Printf("    -H 'Content-Type: application/json' \\")
		log.Printf("    -H 'Authorization: Bearer %s' \\", apiToken)
		log.Printf("    -d '{\"blocks\": [{\"id\": \"test-1\", \"text\": \"Test block\", \"created_at\": \"%s\", \"type\": \"github\", \"datasource\": \"github-main\", \"metadata\": {}}]}'",
			time.Now().Format(time.RFC3339))
		log.Printf("")

		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down importer server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return httpServer.Shutdown(shutdownCtx)
}

func initImporterDB(dbPath string) (*sql.DB, error) {
	db, err := sql.Open("sqlite3", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Apply performance pragmas
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA busy_timeout = 30000",
		"PRAGMA cache_size = -64000",
	}

	for _, pragma := range pragmas {
		if _, err := db.Exec(pragma); err != nil {
			if closeErr := db.Close(); closeErr != nil {
				log.Printf("Warning: failed to close database: %v", closeErr)
			}
			return nil, fmt.Errorf("applying pragma %q: %w", pragma, err)
		}
	}

	// Create schema
	schema := `
		CREATE TABLE IF NOT EXISTS importer_blocks (
			id TEXT PRIMARY KEY,
			target_datasource TEXT NOT NULL,
			block_data TEXT NOT NULL,
			created_at TIMESTAMP NOT NULL,
			imported_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
		);

		CREATE INDEX IF NOT EXISTS idx_target_datasource ON importer_blocks(target_datasource);
		CREATE INDEX IF NOT EXISTS idx_created_at ON importer_blocks(created_at);
	`

	if _, err := db.Exec(schema); err != nil {
		if closeErr := db.Close(); closeErr != nil {
			log.Printf("Warning: failed to close database: %v", closeErr)
		}
		return nil, fmt.Errorf("creating schema: %w", err)
	}

	return db, nil
}

func generateAPIToken() (string, error) {
	// Generate 32 random bytes
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	// Encode as base64 URL-safe string
	return base64.URLEncoding.EncodeToString(b), nil
}

func (s *ImporterServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Health endpoint doesn't require auth
		if r.URL.Path == "/health" {
			next.ServeHTTP(w, r)
			return
		}

		// Check Authorization header
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			s.writeError(w, http.StatusUnauthorized, "missing_auth", "Authorization header required")
			return
		}

		// Expected format: "Bearer <token>"
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != "Bearer" {
			s.writeError(w, http.StatusUnauthorized, "invalid_auth", "Authorization header must be 'Bearer <token>'")
			return
		}

		token := parts[1]
		if token != s.apiToken {
			s.writeError(w, http.StatusUnauthorized, "invalid_token", "Invalid API token")
			return
		}

		// Token is valid, proceed
		next.ServeHTTP(w, r)
	})
}

func (s *ImporterServer) handleImportBlocks(w http.ResponseWriter, r *http.Request) {
	var req ImportBlocksRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		s.writeError(w, http.StatusBadRequest, "invalid_json", fmt.Sprintf("Failed to parse request body: %v", err))
		return
	}

	if len(req.Blocks) == 0 {
		s.writeError(w, http.StatusBadRequest, "empty_request", "No blocks provided")
		return
	}

	// Validate and store blocks
	var errors []string
	accepted := 0
	rejected := 0

	tx, err := s.db.Begin()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to begin transaction: %v", err))
		return
	}

	committed := false
	defer func() {
		if !committed {
			if rbErr := tx.Rollback(); rbErr != nil {
				log.Printf("Warning: failed to rollback transaction: %v", rbErr)
			}
		}
	}()

	stmt, err := tx.Prepare(`
		INSERT OR REPLACE INTO importer_blocks (id, target_datasource, block_data, created_at)
		VALUES (?, ?, ?, ?)
	`)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to prepare statement: %v", err))
		return
	}
	defer func() {
		if err := stmt.Close(); err != nil {
			log.Printf("Warning: failed to close statement: %v", err)
		}
	}()

	for i, block := range req.Blocks {
		// Validate block (matches core.Block interface requirements)
		if block.ID() == "" {
			errors = append(errors, fmt.Sprintf("Block %d: missing ID", i))
			rejected++
			continue
		}
		if block.Text() == "" {
			errors = append(errors, fmt.Sprintf("Block %d (%s): missing text", i, block.ID()))
			rejected++
			continue
		}
		if block.CreatedAt().IsZero() {
			errors = append(errors, fmt.Sprintf("Block %d (%s): missing or invalid created_at", i, block.ID()))
			rejected++
			continue
		}
		if block.Type() == "" {
			errors = append(errors, fmt.Sprintf("Block %d (%s): missing type", i, block.ID()))
			rejected++
			continue
		}
		if block.Source() == "" {
			errors = append(errors, fmt.Sprintf("Block %d (%s): missing datasource", i, block.ID()))
			rejected++
			continue
		}

		// Serialize block data using MarshalJSON
		blockData, err := json.Marshal(&block)
		if err != nil {
			errors = append(errors, fmt.Sprintf("Block %d (%s): failed to serialize: %v", i, block.ID(), err))
			rejected++
			continue
		}

		// Generate unique staging ID
		stagingID := fmt.Sprintf("%s-%s", block.Source(), uuid.New().String())

		// Store in database
		_, err = stmt.Exec(stagingID, block.Source(), string(blockData), block.CreatedAt())
		if err != nil {
			errors = append(errors, fmt.Sprintf("Block %d (%s): failed to store: %v", i, block.ID(), err))
			rejected++
			continue
		}

		accepted++
	}

	if err := tx.Commit(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to commit transaction: %v", err))
		return
	}
	committed = true

	log.Printf("Imported %d blocks (rejected: %d)", accepted, rejected)

	response := ImportBlocksResponse{
		Accepted: accepted,
		Rejected: rejected,
		Errors:   errors,
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *ImporterServer) handleExportBlocks(w http.ResponseWriter, r *http.Request) {
	// Query all blocks
	rows, err := s.db.Query(`
		SELECT id, target_datasource, block_data, created_at
		FROM importer_blocks
		ORDER BY created_at ASC
	`)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to query blocks: %v", err))
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Warning: failed to close rows: %v", err)
		}
	}()

	var blocks []core.GenericBlock
	var blockIDs []string

	for rows.Next() {
		var id, targetDatasource, blockDataJSON string
		var createdAt time.Time

		if err := rows.Scan(&id, &targetDatasource, &blockDataJSON, &createdAt); err != nil {
			log.Printf("Error scanning block row: %v", err)
			continue
		}

		// Parse block data using UnmarshalJSON
		var blockData core.GenericBlock
		if err := json.Unmarshal([]byte(blockDataJSON), &blockData); err != nil {
			log.Printf("Error unmarshaling block data for ID %s: %v", id, err)
			continue
		}

		blocks = append(blocks, blockData)
		blockIDs = append(blockIDs, id)
	}

	if err := rows.Err(); err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to iterate blocks: %v", err))
		return
	}

	// Delete exported blocks atomically
	if len(blockIDs) > 0 {
		tx, err := s.db.Begin()
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to begin transaction: %v", err))
			return
		}

		committed := false
		defer func() {
			if !committed {
				if rbErr := tx.Rollback(); rbErr != nil {
					log.Printf("Warning: failed to rollback transaction: %v", rbErr)
				}
			}
		}()

		stmt, err := tx.Prepare("DELETE FROM importer_blocks WHERE id = ?")
		if err != nil {
			s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to prepare delete: %v", err))
			return
		}
		defer func() {
			if err := stmt.Close(); err != nil {
				log.Printf("Warning: failed to close statement: %v", err)
			}
		}()

		for _, id := range blockIDs {
			if _, err := stmt.Exec(id); err != nil {
				s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to delete block: %v", err))
				return
			}
		}

		if err := tx.Commit(); err != nil {
			s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to commit transaction: %v", err))
			return
		}
		committed = true

		log.Printf("Exported and deleted %d blocks", len(blocks))
	}

	response := map[string]interface{}{
		"blocks": blocks,
		"count":  len(blocks),
	}

	s.writeJSON(w, http.StatusOK, response)
}

func (s *ImporterServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"service":   "importer",
	}

	s.writeJSON(w, http.StatusOK, health)
}

func (s *ImporterServer) handleStats(w http.ResponseWriter, r *http.Request) {
	// Get overall stats
	var totalBlocks int
	err := s.db.QueryRow("SELECT COUNT(*) FROM importer_blocks").Scan(&totalBlocks)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to get stats: %v", err))
		return
	}

	// Get per-datasource stats
	rows, err := s.db.Query(`
		SELECT target_datasource, COUNT(*), MIN(created_at), MAX(created_at)
		FROM importer_blocks
		GROUP BY target_datasource
	`)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "database_error", fmt.Sprintf("Failed to get datasource stats: %v", err))
		return
	}
	defer func() {
		if err := rows.Close(); err != nil {
			log.Printf("Warning: failed to close rows: %v", err)
		}
	}()

	datasourceStats := make(map[string]interface{})
	for rows.Next() {
		var datasource string
		var count int
		var minCreated, maxCreated time.Time

		if err := rows.Scan(&datasource, &count, &minCreated, &maxCreated); err != nil {
			log.Printf("Error scanning datasource stats: %v", err)
			continue
		}

		datasourceStats[datasource] = map[string]interface{}{
			"pending_blocks": count,
			"oldest_block":   minCreated,
			"newest_block":   maxCreated,
		}
	}

	stats := map[string]interface{}{
		"total_pending_blocks": totalBlocks,
		"datasources":          datasourceStats,
	}

	s.writeJSON(w, http.StatusOK, stats)
}

func (s *ImporterServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

func (s *ImporterServer) writeError(w http.ResponseWriter, status int, error, message string) {
	response := ErrorResponse{
		Error:   error,
		Message: message,
	}
	s.writeJSON(w, status, response)
}

func corsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")

		if r.Method == "OPTIONS" {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
