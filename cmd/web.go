package cmd

import (
	"context"
	"embed"
	"fmt"

	"log"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/rubiojr/ergs/cmd/web/components"
	"github.com/rubiojr/ergs/cmd/web/components/types"
	"github.com/rubiojr/ergs/pkg/api"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	renderers "github.com/rubiojr/ergs/pkg/renderers"
	"github.com/rubiojr/ergs/pkg/version"

	"github.com/rubiojr/ergs/pkg/shared"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/urfave/cli/v3"
)

//go:embed web/static/*
var staticFS embed.FS

// WebCommand creates the web command with both API and UI
func WebCommand() *cli.Command {
	return &cli.Command{
		Name:  "web",
		Usage: "Start web server with both API endpoints and HTML interface",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "port",
				Usage: "Port to listen on",
				Value: "8080",
			},
			&cli.StringFlag{
				Name:  "host",
				Usage: "Host to bind to",
				Value: "localhost",
			},
		},
		Action: func(ctx context.Context, c *cli.Command) error {
			return startWebServer(ctx, c.String("config"), c.String("host"), c.String("port"))
		},
	}
}

// WebServer holds the server configuration and dependencies
type WebServer struct {
	registry         *core.Registry
	storageManager   *storage.Manager
	config           *config.Config
	rendererRegistry *renderers.RendererRegistry
	apiServer        *api.Server
}

// startWebServer starts the web server with both API and UI
func startWebServer(ctx context.Context, configPath, host, port string) error {
	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		return fmt.Errorf("loading config: %w", err)
	}

	registry := core.GetGlobalRegistry()

	if err := createDatasourcesFromConfig(registry, cfg); err != nil {
		return fmt.Errorf("creating datasources: %w", err)
	}
	defer func() {
		if err := registry.Close(); err != nil {
			fmt.Printf("Warning: failed to close registry: %v\n", err)
		}
	}()

	storageManager, err := storage.NewManager(cfg.StorageDir)
	if err != nil {
		return fmt.Errorf("creating storage manager: %w", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			fmt.Printf("Warning: failed to close storage manager: %v\n", err)
		}
	}()

	if err := initializeDatasourceStorage(registry, storageManager); err != nil {
		return fmt.Errorf("initializing storage: %w", err)
	}

	// Initialize renderer registry with auto-registered renderers
	rendererRegistry := renderers.GetGlobalRegistry()

	apiServer := api.NewServer(registry, storageManager)

	webServer := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		config:           cfg,
		rendererRegistry: rendererRegistry,
		apiServer:        apiServer,
	}

	mux := http.NewServeMux()

	// API routes
	webServer.apiServer.RegisterRoutes(mux)

	// Web UI routes
	mux.HandleFunc("/", webServer.handleHome)
	mux.HandleFunc("/search", webServer.handleSearch)
	mux.HandleFunc("/firehose", webServer.handleFirehose)
	mux.HandleFunc("/datasources", webServer.handleDatasources)
	mux.HandleFunc("/datasource/", webServer.handleDatasource)

	// Static assets
	mux.HandleFunc("/static/", webServer.handleStatic)

	// Add CORS middleware
	handler := api.CorsMiddleware(mux)

	server := &http.Server{
		Addr:    fmt.Sprintf("%s:%s", host, port),
		Handler: handler,
	}

	// Start server in goroutine
	go func() {
		log.Printf("Starting web server on http://%s:%s", host, port)
		log.Printf("Available endpoints:")
		log.Printf("  Web UI:")
		log.Printf("    GET / - Home page with datasource overview")
		log.Printf("    GET /search - Search across all datasources")
		log.Printf("    GET /firehose - Latest blocks across all datasources")
		log.Printf("    GET /datasources - List all datasources")
		log.Printf("    GET /datasource/{name} - Browse specific datasource")
		log.Printf("  API:")
		log.Printf("    GET /api/datasources - List all datasources")
		log.Printf("    GET /api/datasources/{name} - List blocks from a datasource")
		log.Printf("    GET /api/search - Search across all datasources")
		log.Printf("    GET /api/firehose - Latest blocks across all datasources")
		log.Printf("    GET /api/stats - Get storage statistics")
		log.Printf("    GET /health - Health check")

		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("Server failed to start: %v", err)
		}
	}()

	// Wait for interrupt signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	log.Println("Shutting down web server...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	return server.Shutdown(shutdownCtx)
}

// Web UI Handlers

// handleHome serves the main page
// handleHome handles the home page
func (s *WebServer) handleHome(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}

	// Check if there's a search query
	query := r.URL.Query().Get("q")
	if query != "" {
		// Redirect to search page
		http.Redirect(w, r, "/search?q="+query, http.StatusFound)
		return
	}

	datasources := shared.GetDatasourceList(s.registry, s.storageManager)

	// Calculate additional stats
	var totalBlocks int
	var activeDatasources int
	var oldestBlock, newestBlock *time.Time

	for _, ds := range datasources {
		// Skip importer (router) datasource entirely
		if ds.Type == "importer" {
			continue
		}

		if ds.Stats != nil {
			if blocks, ok := ds.Stats["total_blocks"].(int); ok && blocks > 0 {
				totalBlocks += blocks
				activeDatasources++

				if oldest, ok := ds.Stats["oldest_block"].(time.Time); ok {
					if oldestBlock == nil || oldest.Before(*oldestBlock) {
						oldestBlock = &oldest
					}
				}
				if newest, ok := ds.Stats["newest_block"].(time.Time); ok {
					if newestBlock == nil || newest.After(*newestBlock) {
						newestBlock = &newest
					}
				}
			}
		}
	}

	data := types.PageData{
		Title:             "Ergs - Data Explorer",
		Datasources:       datasources,
		TotalBlocks:       totalBlocks,
		ActiveDatasources: activeDatasources,
		OldestBlock:       oldestBlock,
		NewestBlock:       newestBlock,
		Version:           version.APIVersion(),
	}

	if err := components.Index(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// handleSearch handles search requests with distributed results
func (s *WebServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	// Parse search parameters using search service
	params, err := storage.ParseSearchParams(r.URL.Query())
	if err != nil {
		http.Error(w, fmt.Sprintf("Invalid parameters: %v", err), http.StatusBadRequest)
		return
	}

	// Apply web-specific limit constraint
	if params.Limit > 100 {
		params.Limit = 100
	}

	data := types.PageData{
		Title:               "Search - Ergs",
		Query:               params.Query,
		SelectedDatasources: params.DatasourceFilters,
		Datasources:         shared.GetDatasourceList(s.registry, s.storageManager),
		CurrentPage:         params.Page,
		PageSize:            params.Limit,
		StartDate:           params.StartDate,
		EndDate:             params.EndDate,
		Version:             version.APIVersion(),
	}

	// Web allows empty queries (shows search page)
	if params.Query != "" {
		// Perform search using search service
		searchService := s.storageManager.GetSearchService()
		results, err := searchService.Search(params)
		if err != nil {
			// Handle search errors gracefully instead of returning HTTP 500
			data.Error = formatSearchError(err)
			// Still render the search page with the error message
		} else {
			data.Results = s.convertBlocksToWebBlocks(results.Results)
			data.TotalCount = results.TotalCount
			data.HasNextPage = results.HasMore
			data.TotalPages = results.TotalPages
		}
	}

	if err := components.Search(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// handleDatasources handles the datasources listing page
func (s *WebServer) handleDatasources(w http.ResponseWriter, r *http.Request) {
	allDatasources := shared.GetDatasourceList(s.registry, s.storageManager)

	data := types.PageData{
		Title:       "Datasources - Ergs",
		Datasources: allDatasources,
		Version:     version.APIVersion(),
	}

	if err := components.Datasources(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// handleDatasource handles individual datasource browsing with pagination
func (s *WebServer) handleDatasource(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if len(path) < len("/datasource/") {
		http.NotFound(w, r)
		return
	}

	datasourceName := path[len("/datasource/"):]
	if datasourceName == "" {
		http.NotFound(w, r)
		return
	}

	// Check if datasource exists
	datasources := s.registry.GetAllDatasources()
	if _, exists := datasources[datasourceName]; !exists {
		http.NotFound(w, r)
		return
	}

	// Parse pagination parameters
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}

	limit := 30 // Fixed limit for datasource browsing

	data := types.PageData{
		Title:       fmt.Sprintf("%s - Ergs", datasourceName),
		Datasource:  datasourceName,
		CurrentPage: page,
		PageSize:    limit,
		Version:     version.APIVersion(),
	}

	// Use search service for consistent pagination behavior
	searchService := s.storageManager.GetSearchService()
	params := storage.SearchParams{
		Query:             "", // Empty query for browsing all blocks
		DatasourceFilters: []string{datasourceName},
		Page:              page,
		Limit:             limit,
	}

	results, err := searchService.Search(params)
	if err != nil {
		data.Error = fmt.Sprintf("Failed to load blocks: %v", err)
	} else {
		blocks, exists := results.Results[datasourceName]
		if !exists {
			blocks = []core.Block{}
		}

		// Convert to web blocks
		webBlocks := make([]types.WebBlock, len(blocks))
		for i, block := range blocks {
			webBlocks[i] = s.convertBlockToWebBlock(block)
		}

		data.Results = map[string][]types.WebBlock{datasourceName: webBlocks}
		data.TotalCount = results.TotalCount
		data.HasNextPage = results.HasMore
		data.TotalPages = results.TotalPages
	}

	if err := components.Datasource(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// handleFirehose handles the firehose page showing latest blocks across all datasources
func (s *WebServer) handleFirehose(w http.ResponseWriter, r *http.Request) {
	// Parse pagination parameters
	pageStr := r.URL.Query().Get("page")
	page := 1
	if pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}

	limitStr := r.URL.Query().Get("limit")
	limit := 30 // Default limit for firehose
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	data := types.PageData{
		Title:       "Firehose - Ergs",
		CurrentPage: page,
		PageSize:    limit,
		Version:     version.APIVersion(),
	}

	// Use search service to get all blocks across datasources
	searchService := s.storageManager.GetSearchService()
	params := storage.SearchParams{
		Query: "", // Empty query to get all blocks
		Page:  page,
		Limit: limit,
	}

	results, err := searchService.Search(params)
	if err != nil {
		data.Error = fmt.Sprintf("Failed to load firehose: %v", err)
	} else {
		// Convert to web blocks
		data.Results = s.convertBlocksToWebBlocks(results.Results)
		data.TotalCount = results.TotalCount
		data.HasNextPage = results.HasMore
		data.TotalPages = results.TotalPages
	}

	if err := components.Firehose(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// handleStatic serves static assets from embedded files
func (s *WebServer) handleStatic(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path

	// Remove /static/ prefix and add web/static/ prefix for embedded filesystem
	filePath := "web/static/" + strings.TrimPrefix(path, "/static/")

	// Read file from embedded filesystem
	content, err := staticFS.ReadFile(filePath)
	if err != nil {
		http.NotFound(w, r)
		return
	}

	// Set appropriate content type
	if strings.HasSuffix(path, ".css") {
		w.Header().Set("Content-Type", "text/css")
	} else if strings.HasSuffix(path, ".js") {
		w.Header().Set("Content-Type", "application/javascript")
	} else if strings.HasSuffix(path, ".html") {
		w.Header().Set("Content-Type", "text/html")
	} else if strings.HasSuffix(path, ".ico") {
		w.Header().Set("Content-Type", "image/x-icon")
	} else if strings.HasSuffix(path, ".png") {
		w.Header().Set("Content-Type", "image/png")
	}

	// Set cache headers for static assets
	w.Header().Set("Cache-Control", "public, max-age=3600")

	if _, err := w.Write(content); err != nil {
		log.Printf("Error writing static content: %v", err)
	}
}

// Helper methods

// convertBlocksToWebBlocks converts core blocks to web blocks
func (s *WebServer) convertBlocksToWebBlocks(results map[string][]core.Block) map[string][]types.WebBlock {
	webResults := make(map[string][]types.WebBlock)

	for datasource, blocks := range results {
		webBlocks := make([]types.WebBlock, len(blocks))
		for i, block := range blocks {
			webBlocks[i] = s.convertBlockToWebBlock(block)
		}
		webResults[datasource] = webBlocks
	}

	return webResults
}

// convertBlockToWebBlock converts a core block to a web block using custom renderers
func (s *WebServer) convertBlockToWebBlock(block core.Block) types.WebBlock {
	webBlock := types.WebBlock{
		ID:        block.ID(),
		Text:      block.Text(),
		Source:    block.Source(),
		CreatedAt: block.CreatedAt(),
		Metadata:  block.Metadata(),
		Links:     extractLinks(block.Text()),
	}

	// Use the renderer registry to get properly formatted HTML
	webBlock.FormattedText = string(s.rendererRegistry.Render(block))

	return webBlock
}

// extractLinks extracts HTTP/HTTPS URLs from text
func extractLinks(text string) []string {
	var links []string
	words := strings.Fields(text)

	for _, word := range words {
		if strings.HasPrefix(word, "http://") || strings.HasPrefix(word, "https://") {
			// Clean up punctuation at the end
			cleaned := strings.TrimRight(word, ".,!?;:")
			links = append(links, cleaned)
		}
	}

	return links
}

// formatSearchError converts search errors into user-friendly messages
func formatSearchError(err error) string {
	errStr := err.Error()

	// Handle FTS5 syntax errors
	if strings.Contains(errStr, "fts5: syntax error") {
		if strings.Contains(errStr, "syntax error near \"/\"") {
			return "Invalid search query: Forward slashes (/) are not allowed in search terms. Please remove special characters or quote the query and try again."
		}
		if strings.Contains(errStr, "syntax error near \"'\"") {
			return "Invalid search query: Unmatched single quotes detected. Please use double quotes for phrase searches or remove single quotes."
		}
		if strings.Contains(errStr, "syntax error") {
			return "Invalid search syntax. Please check your query for special characters, unmatched quotes, or invalid operators."
		}
	}

	// Handle other SQLite errors
	if strings.Contains(errStr, "SQL logic error") {
		return "Search query contains invalid syntax. Please simplify your query and try again."
	}

	// Handle database connectivity issues
	if strings.Contains(errStr, "database is locked") {
		return "Database is temporarily busy. Please try again in a moment."
	}

	// Handle generic search errors
	if strings.Contains(errStr, "searching") {
		return "Search error occurred. Please check your query syntax and try again."
	}

	// Fallback for unknown errors - show a generic message
	return "Search failed due to an unexpected error. Please try a simpler query."
}

// writeJSON writes a JSON response
