// Package api provides HTTP handlers for the Ergs REST API endpoints.
// This package implements the HTTP handlers that serve JSON responses for
// datasource management, search functionality, and system statistics.
package api

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"

	"github.com/rubiojr/ergs/pkg/version"
)

// HandleListDatasources handles GET /api/datasources requests.
// It returns a JSON response containing all configured datasources with their statistics.
//
// Response format:
//
//	{
//	  "datasources": [
//	    {
//	      "name": "github",
//	      "type": "github",
//	      "stats": {
//	        "total_blocks": 150,
//	        "last_updated": "2024-01-15T10:30:00Z"
//	      }
//	    }
//	  ],
//	  "count": 1
//	}
//
// The handler returns HTTP 200 on success with a ListDatasourcesResponse JSON body.
// Returns HTTP 500 if there's an internal error retrieving datasource information.
func (s *Server) HandleListDatasources(w http.ResponseWriter, r *http.Request) {
	datasourceInfos := s.getDatasourceList()

	response := ListDatasourcesResponse{
		Datasources: datasourceInfos,
		Count:       len(datasourceInfos),
	}

	s.writeJSON(w, http.StatusOK, response)
}

// HandleDatasourceBlocks handles GET /api/datasources/{name} requests.
// It returns blocks from a specific datasource with optional search and pagination.
//
// Path parameters:
//   - name: The datasource name (required)
//
// Query parameters:
//   - q: Optional search query string for full-text search within the datasource
//   - limit: Maximum number of results (default: 20, max: 100)
//   - page: Page number for pagination (default: 1)
//   - start_date: Date in YYYY-MM-DD format to filter blocks created on or after this date
//   - end_date: Date in YYYY-MM-DD format to filter blocks created on or before this date (inclusive of entire day)
//
// Response format:
//
//	{
//	  "datasource": "github",
//	  "blocks": [
//	    {
//	      "id": "abc123",
//	      "text": "Fixed critical bug in authentication module",
//	      "source": "https://github.com/user/repo/issues/123",
//	      "created_at": "2024-01-15T10:30:00Z",
//	      "metadata": {
//	        "author": "john.doe",
//	        "labels": ["bug", "critical"]
//	      }
//	    }
//	  ],
//	  "count": 1,
//	  "query": "bug fix"
//	}
//
// Returns:
//   - HTTP 200: Success with ListBlocksResponse JSON body
//   - HTTP 400: Invalid request (bad query parameters, invalid date format)
//   - HTTP 404: Datasource not found
//   - HTTP 500: Internal server error
func (s *Server) HandleDatasourceBlocks(w http.ResponseWriter, r *http.Request) {
	// Extract datasource name from path parameter
	datasourceName := r.PathValue("name")
	if datasourceName == "" {
		s.writeError(w, http.StatusBadRequest, "Invalid path", "Datasource name is required")
		return
	}

	// Check if datasource exists
	datasources := s.registry.GetAllDatasources()
	if _, exists := datasources[datasourceName]; !exists {
		s.writeError(w, http.StatusNotFound, "Datasource not found", fmt.Sprintf("Datasource '%s' does not exist", datasourceName))
		return
	}

	// Parse parameters using search service
	params, err := storage.ParseSearchParams(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid date format", err.Error())
		return
	}

	// Override defaults for datasource-specific search
	if params.Limit == 30 {
		params.Limit = 20 // Default for datasource endpoint
	}
	params.DatasourceFilters = []string{datasourceName}

	// Perform search using search service
	searchService := s.storageManager.GetSearchService()
	results, err := searchService.Search(params)
	if err != nil {
		// Handle FTS5 syntax errors more gracefully
		if isFTS5SyntaxError(err) {
			s.writeError(w, http.StatusBadRequest, "Invalid search query", formatAPISearchError(err))
		} else {
			s.writeError(w, http.StatusInternalServerError, "Failed to retrieve blocks", err.Error())
		}
		return
	}

	// Get blocks for this datasource
	blocks, exists := results.Results[datasourceName]
	if !exists {
		blocks = []core.Block{}
	}

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
		Query:      params.Query,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// HandleSearch handles GET /api/search requests.
// It performs full-text search across all configured datasources simultaneously.
//
// Query parameters:
//   - q: Search query string (required) - supports full-text search with FTS5 syntax
//   - limit: Maximum number of results per datasource (default: 30, max: 100)
//   - page: Page number for pagination (default: 1)
//   - start_date: Date in YYYY-MM-DD format to filter blocks created on or after this date
//   - end_date: Date in YYYY-MM-DD format to filter blocks created on or before this date (inclusive of entire day)
//   - datasources: Comma-separated list of datasource names to limit search scope
//
// Search query syntax:
//   - Simple terms: "authentication bug"
//   - Exact phrases: "exact phrase" (use double quotes)
//   - Multiple terms: All terms must be present (AND logic)
//   - Case insensitive matching
//   - Partial word matching supported
//
// Response format:
//
//	{
//	  "query": "authentication",
//	  "results": {
//	    "github": {
//	      "datasource": "github",
//	      "blocks": [...],
//	      "count": 1,
//	      "query": "authentication"
//	    },
//	    "notes": {
//	      "datasource": "notes",
//	      "blocks": [...],
//	      "count": 1,
//	      "query": "authentication"
//	    }
//	  },
//	  "total_count": 2,
//	  "page": 1,
//	  "limit": 30,
//	  "total_pages": 1,
//	  "has_more": false
//	}
//
// Returns:
//   - HTTP 200: Success with SearchResponse JSON body
//   - HTTP 400: Bad request (missing query parameter, invalid search syntax, invalid dates)
//   - HTTP 500: Internal server error
func (s *Server) HandleSearch(w http.ResponseWriter, r *http.Request) {
	// Parse search parameters
	params, err := storage.ParseSearchParams(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid date format", err.Error())
		return
	}

	// API requires a query parameter
	if params.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Missing query parameter", "Query parameter 'q' is required")
		return
	}

	// Perform search using search service
	searchService := s.storageManager.GetSearchService()
	results, err := searchService.Search(params)
	if err != nil {
		// Handle FTS5 syntax errors more gracefully
		if isFTS5SyntaxError(err) {
			s.writeError(w, http.StatusBadRequest, "Invalid search query", formatAPISearchError(err))
		} else {
			s.writeError(w, http.StatusInternalServerError, "Search failed", err.Error())
		}
		return
	}

	// Convert to API response format
	searchResults := make(map[string]ListBlocksResponse)
	for datasourceName, blocks := range results.Results {
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
			Query:      results.Query,
		}
	}

	response := SearchResponse{
		Query:      results.Query,
		Results:    searchResults,
		TotalCount: results.TotalCount,
		Page:       results.Page,
		Limit:      results.Limit,
		TotalPages: results.TotalPages,
		HasMore:    results.HasMore,
	}

	s.writeJSON(w, http.StatusOK, response)
}

// HandleStats handles GET /api/stats requests.
// It returns storage statistics for all configured datasources including
// block counts, storage sizes, and last update timestamps.
//
// Response format:
//
//	{
//	  "github": {
//	    "total_blocks": 150,
//	    "last_updated": "2024-01-15T10:30:00Z",
//	    "storage_size": "2.5MB"
//	  },
//	  "notes": {
//	    "total_blocks": 45,
//	    "storage_size": "1.2MB"
//	  },
//	  "total_blocks": 195,
//	  "total_datasources": 2
//	}
//
// Returns:
//   - HTTP 200: Success with statistics JSON object
//   - HTTP 500: Internal server error if statistics cannot be retrieved
func (s *Server) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.storageManager.GetStats()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get stats", err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

// HandleHealth handles GET /health requests.
// It provides a simple health check endpoint to verify the service is running.
// This endpoint is typically used by load balancers, monitoring systems,
// and container orchestrators to check service availability.
//
// Response format:
//
//	{
//	  "status": "ok",
//	  "timestamp": "2024-01-15T12:00:00Z",
//	  "version": "1.0.0"
//	}
//
// Returns:
//   - HTTP 200: Service is healthy with HealthResponse JSON body
//
// This endpoint always returns HTTP 200 unless there's a critical system failure
// preventing the handler from executing.
func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Version:   version.APIVersion(),
	}

	s.writeJSON(w, http.StatusOK, health)
}

// isFTS5SyntaxError checks if an error is due to FTS5 (Full-Text Search) syntax issues.
// SQLite FTS5 can generate various syntax errors when users provide malformed search queries.
// This function identifies common FTS5 error patterns to provide better user feedback.
//
// Common FTS5 syntax errors include:
//   - Invalid characters like forward slashes (/)
//   - Unmatched quotes
//   - Invalid boolean operators
//   - Malformed phrase queries
//
// Parameters:
//   - err: The error to check for FTS5 syntax issues
//
// Returns:
//   - true if the error appears to be an FTS5 syntax error
//   - false for other types of errors (network, database lock, etc.)
func isFTS5SyntaxError(err error) bool {
	errStr := err.Error()
	return strings.Contains(errStr, "fts5: syntax error") ||
		strings.Contains(errStr, "SQL logic error")
}

// formatAPISearchError converts internal search errors into user-friendly API error messages.
// This function transforms technical SQLite/FTS5 error messages into actionable feedback
// that API consumers can understand and act upon.
//
// Error transformations:
//   - FTS5 forward slash errors → "Forward slashes (/) are not allowed in search terms"
//   - Unmatched quote errors → "Unmatched single quotes detected. Use double quotes for phrase searches"
//   - General syntax errors → "Invalid search syntax. Check for special characters or invalid operators"
//   - SQL logic errors → "Search query contains invalid syntax"
//   - Unknown errors → "Invalid search query format"
//
// Parameters:
//   - err: The original error from the search operation
//
// Returns:
//   - A user-friendly error message suitable for API responses
//
// This function helps maintain API usability by hiding internal implementation details
// while providing actionable guidance to fix search query issues.
func formatAPISearchError(err error) string {
	errStr := err.Error()

	// Handle FTS5 syntax errors
	if strings.Contains(errStr, "fts5: syntax error") {
		if strings.Contains(errStr, "syntax error near \"/\"") {
			return "Forward slashes (/) are not allowed in search terms"
		}
		if strings.Contains(errStr, "syntax error near \"'\"") {
			return "Unmatched single quotes detected. Use double quotes for phrase searches"
		}
		return "Invalid search syntax. Check for special characters or invalid operators"
	}

	// Handle other SQLite errors
	if strings.Contains(errStr, "SQL logic error") {
		return "Search query contains invalid syntax"
	}

	// Fallback for unknown errors
	return "Invalid search query format"
}
