// Package storage provides search functionality across all datasources
// in the Ergs system. SearchService handles parameter parsing, search execution,
// and result formatting for both API and web interfaces.
//
// The search service consolidates all search operations into a single interface,
// eliminating duplication between Manager and GenericStorage while providing
// multi-datasource searching, pagination, date filtering, and result aggregation.
package storage

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

// SearchParams represents all parameters for a search operation.
// It provides a unified structure for search configuration that works
// across both API and web interfaces.
type SearchParams struct {
	// Query is the search term to look for across all datasources.
	// Can be empty for web interface (shows search page without results).
	Query string

	// DatasourceFilters limits the search to specific datasource instances.
	// If empty, searches across all available datasources.
	// Example: ["github", "rss", "firefox"]
	DatasourceFilters []string

	// Page is the page number for pagination (1-based).
	// Defaults to 1 if not specified.
	Page int

	// Limit is the maximum number of results per page.
	// Defaults to 30 if not specified.
	Limit int

	// StartDate filters results to items created on or after this date.
	// If nil, no start date filtering is applied.
	StartDate *time.Time

	// EndDate filters results to items created on or before this date.
	// Automatically set to end of day (23:59:59) when parsed from date string.
	// If nil, no end date filtering is applied.
	EndDate *time.Time
}

// SearchResults contains the results of a search operation along with
// pagination metadata. Results are grouped by datasource for efficient
// display and processing.
type SearchResults struct {
	// Results contains blocks grouped by datasource name.
	// Key is datasource instance name, value is slice of matching blocks.
	Results map[string][]core.Block

	// TotalCount is the number of results returned on this page.
	// Note: This is NOT the total across all pages due to distributed search complexity.
	TotalCount int

	// HasMore indicates whether there are more results available on subsequent pages.
	HasMore bool

	// TotalPages is the estimated total number of pages.
	// This is a conservative estimate based on current page and HasMore status.
	TotalPages int

	// Page is the current page number (matches input parameter).
	Page int

	// Limit is the maximum results per page (matches input parameter).
	Limit int

	// Query is the search term used (matches input parameter).
	Query string
}

// SearchService provides search functionality across all datasources.
// It encapsulates the storage manager and provides a clean interface
// for executing searches with various filters and pagination.
type SearchService struct {
	manager *Manager
}

// NewSearchService creates a new search service with the provided storage manager.
// The storage manager is used to execute searches across all configured datasources.
//
// Parameters:
//   - manager: The storage manager that provides access to all datasource databases
//
// Returns:
//   - *SearchService: A new search service instance ready to execute searches
func NewSearchService(manager *Manager) *SearchService {
	return &SearchService{
		manager: manager,
	}
}

// Search executes a search operation with the provided parameters.
// It handles multi-datasource searching, pagination, date filtering,
// and result aggregation automatically.
//
// The search operation:
// 1. Applies datasource filtering if specified
// 2. Applies date range filtering if specified
// 3. Executes paginated search across target datasources
// 4. Aggregates results and calculates pagination metadata
//
// Parameters:
//   - params: SearchParams containing all search configuration
//
// Returns:
//   - *SearchResults: Results grouped by datasource with pagination metadata
//   - error: Any error that occurred during the search operation
//
// Example:
//
//	params := SearchParams{
//		Query: "golang",
//		DatasourceFilters: []string{"github", "rss"},
//		Page: 1,
//		Limit: 20,
//	}
//	results, err := searchService.Search(params)
func (s *SearchService) Search(params SearchParams) (*SearchResults, error) {
	results, totalResults, hasMoreResults, totalPages, err := s.executeSearch(params)
	if err != nil {
		return nil, err
	}

	return &SearchResults{
		Results:    results,
		TotalCount: totalResults,
		HasMore:    hasMoreResults,
		TotalPages: totalPages,
		Page:       params.Page,
		Limit:      params.Limit,
		Query:      params.Query,
	}, nil
}

// executeSearch performs the actual search operation with all parameters.
func (s *SearchService) executeSearch(params SearchParams) (map[string][]core.Block, int, bool, int, error) {
	// Determine which datasources to search
	datasources := params.DatasourceFilters
	if len(datasources) == 0 {
		datasources = s.manager.SearchAllDatasources()
	}

	// Filter to only include datasources that actually exist
	validDatasources := make([]string, 0, len(datasources))
	s.manager.mu.RLock()
	for _, name := range datasources {
		if _, exists := s.manager.storages[name]; exists {
			validDatasources = append(validDatasources, name)
		}
	}
	s.manager.mu.RUnlock()

	if len(validDatasources) == 0 {
		return make(map[string][]core.Block), 0, false, 1, nil
	}

	// Get enough results to support paging - fetch more than needed from each datasource
	// Fetch extra to determine if there are more pages
	requestLimit := (params.Page + 1) * params.Limit
	results := s.searchDatasourcesInParallel(validDatasources, params.Query, requestLimit, true, params.StartDate, params.EndDate)

	// Check for any errors first - fail fast like original behavior
	for _, result := range results {
		if result.err != nil {
			return nil, 0, false, 0, fmt.Errorf("searching %s: %w", result.datasource, result.err)
		}
	}

	// Sort datasources by the creation time of their newest block
	sortedDatasources := s.manager.sortDatasourcesByNewestBlock(s.convertResultsToMap(results))

	// Flatten results in datasource order (ordered by newest block per datasource)
	var allBlocks []core.Block
	var blockToDatasource []string

	for _, dsName := range sortedDatasources {
		for _, result := range results {
			if result.datasource == dsName {
				for _, block := range result.blocks {
					allBlocks = append(allBlocks, block)
					blockToDatasource = append(blockToDatasource, dsName)
				}
				break
			}
		}
	}

	// Apply pagination to the flattened list
	startIndex := (params.Page - 1) * params.Limit
	endIndex := startIndex + params.Limit

	if startIndex >= len(allBlocks) {
		return make(map[string][]core.Block), 0, false, params.Page, nil
	}

	if endIndex > len(allBlocks) {
		endIndex = len(allBlocks)
	}

	// Group the paginated results back by datasource
	groupedResults := make(map[string][]core.Block)
	for i := startIndex; i < endIndex; i++ {
		dsName := blockToDatasource[i]
		block := allBlocks[i]
		convertedBlocks, err := s.manager.convertBlocksToProperTypes([]core.Block{block})
		if err != nil {
			continue
		}
		if len(convertedBlocks) > 0 {
			groupedResults[dsName] = append(groupedResults[dsName], convertedBlocks[0])
		}
	}

	totalResults := 0
	for _, blocks := range groupedResults {
		totalResults += len(blocks)
	}

	if totalResults == 0 && params.Page > 1 {
		return make(map[string][]core.Block), 0, false, params.Page, nil
	}

	// Check if there are more results by looking at remaining blocks beyond the current page
	hasMoreResults := endIndex < len(allBlocks)

	totalPages := params.Page
	if hasMoreResults {
		totalPages = params.Page + 1
	}

	// If we didn't get a full page but we're still on page 1, no more results
	if totalResults == 0 && params.Page > 1 {
		totalPages = params.Page
	}

	return groupedResults, totalResults, hasMoreResults, totalPages, nil
}

// convertResultsToMap converts search results to a map for sorting
func (s *SearchService) convertResultsToMap(results []searchResult) map[string][]core.Block {
	resultMap := make(map[string][]core.Block)
	for _, result := range results {
		if result.err == nil {
			resultMap[result.datasource] = result.blocks
		}
	}
	return resultMap
}

// searchDatasourcesInParallel executes searches across multiple datasources in parallel.
func (s *SearchService) searchDatasourcesInParallel(datasources []string, query string, limit int, orderByTime bool, startDate, endDate *time.Time) []searchResult {
	resultChan := make(chan searchResult, len(datasources))

	for _, datasource := range datasources {
		go func(ds string) {
			storage, err := s.manager.GetStorage(ds)
			if err != nil {
				resultChan <- searchResult{datasource: ds, err: err}
				return
			}

			blocks, err := s.executeStorageSearch(storage, query, limit, orderByTime, startDate, endDate)
			resultChan <- searchResult{
				datasource: ds,
				blocks:     blocks,
				err:        err,
			}
		}(datasource)
	}

	var results []searchResult
	for i := 0; i < len(datasources); i++ {
		results = append(results, <-resultChan)
	}

	return results
}

// executeStorageSearch performs the actual database search with all parameters.
func (s *SearchService) executeStorageSearch(storage *GenericStorage, query string, limit int, orderByTime bool, startDate, endDate *time.Time) ([]core.Block, error) {
	var sqlQuery string
	var args []interface{}

	// Build the date range conditions
	var dateConditions []string
	if startDate != nil {
		dateConditions = append(dateConditions, "b.created_at >= ?")
		args = append(args, startDate.Format(time.RFC3339))
	}
	if endDate != nil {
		dateConditions = append(dateConditions, "b.created_at <= ?")
		args = append(args, endDate.Format(time.RFC3339))
	}

	var whereClause string
	if len(dateConditions) > 0 {
		whereClause = " AND " + strings.Join(dateConditions, " AND ")
	}

	if query != "" {
		// Escape FTS5 query for special characters
		escapedQuery := escapeFTS5Query(query)
		orderClause := "ORDER BY b.created_at DESC"
		if !orderByTime {
			orderClause = "ORDER BY bm25(blocks_fts), b.created_at DESC"
		}
		sqlQuery = `
			SELECT b.id, b.text, b.created_at, b.source, b.datasource, b.metadata, b.hostname
			FROM blocks b
			JOIN blocks_fts fts ON b.rowid = fts.rowid
			WHERE blocks_fts MATCH ?` + whereClause + `
			` + orderClause + `
			LIMIT ?`
		args = append([]interface{}{escapedQuery}, args...)
		args = append(args, limit)
	} else {
		if len(dateConditions) > 0 {
			whereClause = " WHERE " + strings.Join(dateConditions, " AND ")
		}
		sqlQuery = `
			SELECT id, text, created_at, source, datasource, metadata, hostname
			FROM blocks` + whereClause + `
			ORDER BY created_at DESC
			LIMIT ?`
		args = append(args, limit)
	}

	rows, err := storage.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("querying blocks: %w", err)
	}
	defer func() {
		if err := rows.Close(); err != nil {
			fmt.Printf("Warning: failed to close rows: %v\n", err)
		}
	}()

	var blocks []core.Block
	for rows.Next() {
		var id, text, source, datasourceType, metadataStr string
		var hostname sql.NullString
		var createdAt time.Time

		err = rows.Scan(&id, &text, &createdAt, &source, &datasourceType, &metadataStr, &hostname)
		if err != nil {
			return nil, fmt.Errorf("scanning row: %w", err)
		}

		var metadata map[string]interface{}
		if err := json.Unmarshal([]byte(metadataStr), &metadata); err != nil {
			return nil, fmt.Errorf("unmarshaling metadata for block %s: %w", id, err)
		}

		hostnameStr := ""
		if hostname.Valid {
			hostnameStr = hostname.String
		}

		block := core.NewGenericBlockWithHostname(id, text, source, datasourceType, hostnameStr, createdAt, metadata)
		blocks = append(blocks, block)
	}

	return blocks, rows.Err()
}

// ParseSearchParams parses HTTP query parameters into a SearchParams struct.
// It handles parameter validation, type conversion, and provides sensible defaults
// for missing or invalid parameters.
//
// Supported parameters:
//   - q: Search query string
//   - datasource: Datasource filter (can be specified multiple times)
//   - page: Page number (positive integer, defaults to 1)
//   - limit: Results per page (positive integer, defaults to 30)
//   - start_date: Start date filter in YYYY-MM-DD format
//   - end_date: End date filter in YYYY-MM-DD format (set to end of day)
//
// Date parsing:
//   - Invalid date formats return an error
//   - End dates are automatically set to 23:59:59 of the specified day
//   - Missing dates are treated as no filtering
//
// Parameters:
//   - queryParams: HTTP query parameters as parsed by net/url
//
// Returns:
//   - SearchParams: Parsed and validated search parameters
//   - error: Error if date parameters are invalid
//
// Example:
//
//	params, err := ParseSearchParams(r.URL.Query())
//	if err != nil {
//		// Handle invalid date format
//	}
func ParseSearchParams(queryParams map[string][]string) (SearchParams, error) {
	params := SearchParams{
		Page:  1,
		Limit: 30,
	}

	// Get query
	if q := queryParams["q"]; len(q) > 0 {
		params.Query = q[0]
	}

	// Get datasource filters
	if datasources := queryParams["datasource"]; len(datasources) > 0 {
		params.DatasourceFilters = datasources
	}

	// Parse limit
	if limitStr := queryParams["limit"]; len(limitStr) > 0 && limitStr[0] != "" {
		if parsed, err := strconv.Atoi(limitStr[0]); err == nil && parsed > 0 {
			params.Limit = parsed
		}
	}

	// Parse page
	if pageStr := queryParams["page"]; len(pageStr) > 0 && pageStr[0] != "" {
		if parsed, err := strconv.Atoi(pageStr[0]); err == nil && parsed > 0 {
			params.Page = parsed
		}
	}

	// Parse start date
	if startDateStr := queryParams["start_date"]; len(startDateStr) > 0 && startDateStr[0] != "" {
		if parsed, err := time.Parse("2006-01-02", startDateStr[0]); err == nil {
			params.StartDate = &parsed
		} else {
			return params, err
		}
	}

	// Parse end date
	if endDateStr := queryParams["end_date"]; len(endDateStr) > 0 && endDateStr[0] != "" {
		if parsed, err := time.Parse("2006-01-02", endDateStr[0]); err == nil {
			endOfDay := time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 23, 59, 59, 999999999, parsed.Location())
			params.EndDate = &endOfDay
		} else {
			return params, err
		}
	}

	return params, nil
}

// searchResult represents the result of searching a single datasource.
type searchResult struct {
	datasource string
	blocks     []core.Block
	err        error
}

// escapeFTS5Query prevents SQL injection while allowing all FTS5 syntax
func escapeFTS5Query(query string) string {
	// The query is used in a parameterized query with MATCH ?,
	// so SQL injection is already prevented by SQLite's parameter binding.
	// We just need to return the query as-is to allow full FTS5 syntax.
	return query
}
