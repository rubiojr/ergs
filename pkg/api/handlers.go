package api

import (
	"fmt"
	"net/http"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/search"
	"github.com/rubiojr/ergs/pkg/version"
)

func (s *Server) HandleListDatasources(w http.ResponseWriter, r *http.Request) {
	datasourceInfos := s.getDatasourceList()

	response := ListDatasourcesResponse{
		Datasources: datasourceInfos,
		Count:       len(datasourceInfos),
	}

	s.writeJSON(w, http.StatusOK, response)
}

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
	params, err := search.ParseSearchParams(r.URL.Query())
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
	searchService := search.NewSearchService(s.storageManager)
	results, err := searchService.Search(params)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to retrieve blocks", err.Error())
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

func (s *Server) HandleSearch(w http.ResponseWriter, r *http.Request) {
	// Parse search parameters
	params, err := search.ParseSearchParams(r.URL.Query())
	if err != nil {
		s.writeError(w, http.StatusBadRequest, "Invalid date format", err.Error())
		return
	}

	// API requires a query parameter
	if params.Query == "" {
		s.writeError(w, http.StatusBadRequest, "Missing query parameter", "Query parameter 'q' is required")
		return
	}

	// Perform search using shared service
	searchService := search.NewSearchService(s.storageManager)
	results, err := searchService.Search(params)
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Search failed", err.Error())
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

func (s *Server) HandleStats(w http.ResponseWriter, r *http.Request) {
	stats, err := s.storageManager.GetStats()
	if err != nil {
		s.writeError(w, http.StatusInternalServerError, "Failed to get stats", err.Error())
		return
	}

	s.writeJSON(w, http.StatusOK, stats)
}

func (s *Server) HandleHealth(w http.ResponseWriter, r *http.Request) {
	health := HealthResponse{
		Status:    "ok",
		Timestamp: time.Now().UTC(),
		Version:   version.APIVersion(),
	}

	s.writeJSON(w, http.StatusOK, health)
}
