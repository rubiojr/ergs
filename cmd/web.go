package cmd

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"

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
	"github.com/rubiojr/ergs/cmd/web/renderers"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/version"
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
}

// BlockResponse represents a block for API responses
type BlockResponse struct {
	ID        string                 `json:"id"`
	Text      string                 `json:"text"`
	Source    string                 `json:"source"`
	CreatedAt time.Time              `json:"created_at"`
	Metadata  map[string]interface{} `json:"metadata"`
}

// API Response types
type ListDatasourcesResponse struct {
	Datasources []types.DatasourceInfo `json:"datasources"`
	Count       int                    `json:"count"`
}

type ListBlocksResponse struct {
	Datasource string          `json:"datasource"`
	Blocks     []BlockResponse `json:"blocks"`
	Count      int             `json:"count"`
	Query      string          `json:"query,omitempty"`
}

type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
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

	webServer := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		config:           cfg,
		rendererRegistry: rendererRegistry,
	}

	mux := http.NewServeMux()

	// API routes
	mux.HandleFunc("/api/datasources", webServer.handleAPIListDatasources)
	mux.HandleFunc("/api/datasources/", webServer.handleAPIDatasourceBlocks)
	mux.HandleFunc("/api/search", webServer.handleAPISearch)
	mux.HandleFunc("/api/stats", webServer.handleAPIStats)
	mux.HandleFunc("/health", webServer.handleHealth)

	// Web UI routes
	mux.HandleFunc("/", webServer.handleHome)
	mux.HandleFunc("/search", webServer.handleSearch)
	mux.HandleFunc("/datasources", webServer.handleDatasources)
	mux.HandleFunc("/datasource/", webServer.handleDatasource)

	// Static assets
	mux.HandleFunc("/static/", webServer.handleStatic)

	// Add CORS middleware
	handler := corsMiddleware(mux)

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
		log.Printf("    GET /datasources - List all datasources")
		log.Printf("    GET /datasource/{name} - Browse specific datasource")
		log.Printf("  API:")
		log.Printf("    GET /api/datasources - List all datasources")
		log.Printf("    GET /api/datasources/{name} - List blocks from a datasource")
		log.Printf("    GET /api/search - Search across all datasources")
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

// API Handlers

// handleAPIListDatasources handles GET /api/datasources
func (s *WebServer) handleAPIListDatasources(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET method is supported")
		return
	}

	// Use the shared method that already sorts alphabetically
	datasourceInfos := s.getDatasourceList()

	response := ListDatasourcesResponse{
		Datasources: datasourceInfos,
		Count:       len(datasourceInfos),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleAPIDatasourceBlocks handles GET /api/datasources/{name}
func (s *WebServer) handleAPIDatasourceBlocks(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET method is supported")
		return
	}

	// Extract datasource name from URL path
	path := r.URL.Path
	if len(path) < len("/api/datasources/") {
		s.writeError(w, http.StatusBadRequest, "Invalid path", "Datasource name is required")
		return
	}

	datasourceName := path[len("/api/datasources/"):]
	if datasourceName == "" {
		s.writeError(w, http.StatusBadRequest, "Invalid path", "Datasource name is required")
		return
	}

	// Parse query parameters
	query := r.URL.Query().Get("q")
	limitStr := r.URL.Query().Get("limit")

	limit := 20 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	// Check if datasource exists
	datasources := s.registry.GetAllDatasources()
	if _, exists := datasources[datasourceName]; !exists {
		s.writeError(w, http.StatusNotFound, "Datasource not found", fmt.Sprintf("Datasource '%s' does not exist", datasourceName))
		return
	}

	// Get blocks
	blocks, err := s.storageManager.SearchBlocks(datasourceName, query, limit)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve blocks", err.Error())
		return
	}

	// Convert blocks to response format
	blockResponses := make([]BlockResponse, len(blocks))
	for i, block := range blocks {
		blockResponses[i] = BlockResponse{
			ID:        block.ID(),
			Text:      block.Text(),
			Source:    block.Source(),
			CreatedAt: block.CreatedAt(),
			Metadata:  block.Metadata(),
		}
	}

	response := ListBlocksResponse{
		Datasource: datasourceName,
		Blocks:     blockResponses,
		Count:      len(blockResponses),
		Query:      query,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleAPISearch handles GET /api/search
func (s *WebServer) handleAPISearch(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET method is supported")
		return
	}

	query := r.URL.Query().Get("q")
	if query == "" {
		s.writeError(w, http.StatusBadRequest, "Missing query parameter", "Query parameter 'q' is required")
		return
	}

	// Parse pagination parameters
	limitStr := r.URL.Query().Get("limit")
	limit := 30 // default to match web interface
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 {
			limit = parsed
		}
	}

	pageStr := r.URL.Query().Get("page")
	page := 1 // default
	if pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}

	// Parse date filter parameters
	var startDate, endDate *time.Time
	if startDateStr := r.URL.Query().Get("start_date"); startDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", startDateStr); err == nil {
			startDate = &parsed
		} else {
			s.writeError(w, http.StatusBadRequest, "Invalid start_date format", "start_date must be in YYYY-MM-DD format")
			return
		}
	}
	if endDateStr := r.URL.Query().Get("end_date"); endDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", endDateStr); err == nil {
			// Set to end of day
			endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 999999999, parsed.Location())
			endDate = &endOfDay
		} else {
			s.writeError(w, http.StatusBadRequest, "Invalid end_date format", "end_date must be in YYYY-MM-DD format")
			return
		}
	}

	// Parse datasource filters
	datasourceFilters := r.URL.Query()["datasource"]

	// Use the same pagination logic as web interface with date filtering
	results, totalResults, hasMoreResults, totalPages := s.getSearchResultsWithDateRange(query, datasourceFilters, page, limit, startDate, endDate)

	// Convert results to response format
	searchResults := make(map[string]ListBlocksResponse)

	for datasourceName, blocks := range results {
		blockResponses := make([]BlockResponse, len(blocks))
		for i, block := range blocks {
			blockResponses[i] = BlockResponse{
				ID:        block.ID(),
				Text:      block.Text(),
				Source:    block.Source(),
				CreatedAt: block.CreatedAt(),
				Metadata:  block.Metadata(),
			}
		}

		searchResults[datasourceName] = ListBlocksResponse{
			Datasource: datasourceName,
			Blocks:     blockResponses,
			Count:      len(blockResponses),
			Query:      query,
		}
	}

	response := map[string]interface{}{
		"query":       query,
		"results":     searchResults,
		"total_count": totalResults,
		"page":        page,
		"limit":       limit,
		"total_pages": totalPages,
		"has_more":    hasMoreResults,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// handleAPIStats handles GET /api/stats
func (s *WebServer) handleAPIStats(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET method is supported")
		return
	}

	stats, err := s.storageManager.GetStats()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get stats", err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

// handleHealth handles GET /health
func (s *WebServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		s.writeError(w, http.StatusMethodNotAllowed, "Method not allowed", "Only GET method is supported")
		return
	}

	health := map[string]interface{}{
		"status":    "ok",
		"timestamp": time.Now().UTC(),
		"version":   version.APIVersion(),
	}

	s.writeJSON(w, http.StatusOK, health)
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

	datasources := s.getDatasourceList()

	// Calculate additional stats
	var totalBlocks int
	var activeDatasources int
	var oldestBlock, newestBlock *time.Time

	for _, ds := range datasources {
		if ds.Stats != nil {
			if blocks, ok := ds.Stats["total_blocks"].(int); ok && blocks > 0 {
				totalBlocks += blocks
				activeDatasources++

				// Check for oldest and newest blocks
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
	}

	if err := components.Index(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// handleSearch handles search requests with distributed results
func (s *WebServer) handleSearch(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query().Get("q")
	selectedDatasources := r.URL.Query()["datasource"]
	limitStr := r.URL.Query().Get("limit")
	pageStr := r.URL.Query().Get("page")

	// Parse parameters
	limit := 30 // default
	if limitStr != "" {
		if parsed, err := strconv.Atoi(limitStr); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	page := 1 // default
	if pageStr != "" {
		if parsed, err := strconv.Atoi(pageStr); err == nil && parsed > 0 {
			page = parsed
		}
	}

	// Parse date filter parameters
	var startDate, endDate *time.Time
	if startDateStr := r.URL.Query().Get("start_date"); startDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", startDateStr); err == nil {
			startDate = &parsed
		}
	}
	if endDateStr := r.URL.Query().Get("end_date"); endDateStr != "" {
		if parsed, err := time.Parse("2006-01-02", endDateStr); err == nil {
			// Set to end of day
			endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 999999999, parsed.Location())
			endDate = &endOfDay
		}
	}

	data := types.PageData{
		Title:               "Search - Ergs",
		Query:               query,
		SelectedDatasources: selectedDatasources,
		Datasources:         s.getDatasourceList(),
		CurrentPage:         page,
		PageSize:            limit,
		StartDate:           startDate,
		EndDate:             endDate,
	}

	if query != "" {
		// Get search results (distributed or multiple datasources) with date filtering
		results, totalCount, hasNextPage, totalPages := s.getSearchResultsWithDateRange(query, selectedDatasources, page, limit, startDate, endDate)

		data.Results = s.convertBlocksToWebBlocks(results)
		data.TotalCount = totalCount
		data.HasNextPage = hasNextPage
		data.TotalPages = totalPages
	}

	if err := components.Search(data).Render(r.Context(), w); err != nil {
		http.Error(w, fmt.Sprintf("Template error: %v", err), http.StatusInternalServerError)
	}
}

// getSearchResultsWithDateRange searches across specified datasource(s) with efficient pagination and date filtering ordered by time
func (s *WebServer) getSearchResultsWithDateRange(query string, datasourceFilters []string, page, totalLimit int, startDate, endDate *time.Time) (map[string][]core.Block, int, bool, int) {
	var results map[string][]core.Block
	var err error

	// Search either specific datasources or all datasources with date filtering
	if len(datasourceFilters) > 0 {
		if startDate != nil || endDate != nil {
			results, err = s.storageManager.SearchDatasourcesPagedWithDateRange(datasourceFilters, query, totalLimit*page*2, page, totalLimit, startDate, endDate)
		} else {
			results, err = s.storageManager.SearchDatasourcesPaged(datasourceFilters, query, totalLimit*page*2, page, totalLimit)
		}
	} else {
		if startDate != nil || endDate != nil {
			results, err = s.storageManager.SearchAllDatasourcesPagedWithDateRange(query, totalLimit*page*2, page, totalLimit, startDate, endDate)
		} else {
			results, err = s.storageManager.SearchAllDatasourcesPaged(query, totalLimit*page*2, page, totalLimit)
		}
	}
	if err != nil {
		return make(map[string][]core.Block), 0, false, 1
	}

	// Count actual results returned for this page
	totalResults := 0
	for _, blocks := range results {
		totalResults += len(blocks)
	}

	// If we got no results, this page doesn't exist
	if totalResults == 0 && page > 1 {
		return make(map[string][]core.Block), 0, false, page
	}

	// Check if there are more results by trying to get one more result
	hasMoreResults := false
	if totalResults == totalLimit {
		// Try to get the next page to see if it has results
		var nextPageResults map[string][]core.Block
		if len(datasourceFilters) > 0 {
			if startDate != nil || endDate != nil {
				nextPageResults, err = s.storageManager.SearchDatasourcesPagedWithDateRange(datasourceFilters, query, totalLimit*(page+1)*2, page+1, 1, startDate, endDate)
			} else {
				nextPageResults, err = s.storageManager.SearchDatasourcesPaged(datasourceFilters, query, totalLimit*(page+1)*2, page+1, 1)
			}
		} else {
			if startDate != nil || endDate != nil {
				nextPageResults, err = s.storageManager.SearchAllDatasourcesPagedWithDateRange(query, totalLimit*(page+1)*2, page+1, 1, startDate, endDate)
			} else {
				nextPageResults, err = s.storageManager.SearchAllDatasourcesPaged(query, totalLimit*(page+1)*2, page+1, 1)
			}
		}
		if err == nil {
			nextPageCount := 0
			for _, blocks := range nextPageResults {
				nextPageCount += len(blocks)
			}
			hasMoreResults = nextPageCount > 0
		}
	}

	// Calculate total pages - we don't know the exact total, so estimate conservatively
	totalPages := page
	if hasMoreResults {
		totalPages = page + 1 // We know there's at least one more page
	}

	// For pages beyond available results, ensure totalPages >= page
	if totalResults == 0 && page > 1 {
		totalPages = page // This page exists but is empty
	}

	return results, totalResults, hasMoreResults, totalPages
}

// handleDatasources handles the datasources listing page
func (s *WebServer) handleDatasources(w http.ResponseWriter, r *http.Request) {
	allDatasources := s.getDatasourceList()

	data := types.PageData{
		Title:       "Datasources - Ergs",
		Datasources: allDatasources,
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
	}

	// Get blocks with pagination
	offset := (page - 1) * limit
	checkLimit := limit + 1 // Get one extra to check for next page

	// Get blocks from storage (empty query for recent blocks)
	allBlocks, err := s.storageManager.SearchBlocks(datasourceName, "", offset+checkLimit)
	if err != nil {
		data.Error = fmt.Sprintf("Failed to load blocks: %v", err)
	} else {
		// Apply pagination
		var blocks []core.Block
		if offset < len(allBlocks) {
			end := offset + limit
			if end > len(allBlocks) {
				end = len(allBlocks)
			}
			blocks = allBlocks[offset:end]

			// Check if there are more blocks for next page
			data.HasNextPage = len(allBlocks) > offset+limit
		} else {
			blocks = []core.Block{}
		}

		// Convert to web blocks
		webBlocks := make([]types.WebBlock, len(blocks))
		for i, block := range blocks {
			webBlocks[i] = s.convertBlockToWebBlock(block)
		}

		data.Results = map[string][]types.WebBlock{datasourceName: webBlocks}

		// Calculate total pages for datasource browsing
		totalBlocks, err := s.storageManager.SearchBlocks(datasourceName, "", 10000) // Get large number to count total
		totalAvailable := 0
		if err == nil {
			totalAvailable = len(totalBlocks)
		}
		data.TotalCount = totalAvailable
		data.TotalPages = (totalAvailable + limit - 1) / limit
		if data.TotalPages == 0 {
			data.TotalPages = 1
		}
	}

	if err := components.Datasource(data).Render(r.Context(), w); err != nil {
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

// getDatasourceList returns a list of all datasources with stats
func (s *WebServer) getDatasourceList() []types.DatasourceInfo {
	datasources := s.registry.GetAllDatasources()
	datasourceInfos := make([]types.DatasourceInfo, 0, len(datasources))

	stats, _ := s.storageManager.GetStats()

	for name, ds := range datasources {
		info := types.DatasourceInfo{
			Name: name,
			Type: ds.Type(),
		}

		if stats != nil {
			if dsStats, exists := stats[name]; exists {
				info.Stats = dsStats.(map[string]interface{})
			}
		}

		datasourceInfos = append(datasourceInfos, info)
	}

	// Sort datasources alphabetically by name
	sort.Slice(datasourceInfos, func(i, j int) bool {
		return datasourceInfos[i].Name < datasourceInfos[j].Name
	})

	return datasourceInfos
}

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

// writeJSON writes a JSON response
func (s *WebServer) writeJSON(w http.ResponseWriter, status int, data interface{}) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)

	if err := json.NewEncoder(w).Encode(data); err != nil {
		log.Printf("Error encoding JSON response: %v", err)
	}
}

// writeError writes an error response
func (s *WebServer) writeError(w http.ResponseWriter, status int, error, message string) {
	response := ErrorResponse{
		Error:   error,
		Message: message,
	}
	s.writeJSON(w, status, response)
}

// corsMiddleware adds CORS headers to responses
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
