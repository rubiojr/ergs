// Package search provides a unified interface for searching across all datasources
// in the Ergs system. It handles parameter parsing, search execution, and result
// formatting for both API and web interfaces.
//
// The search service abstracts the complexity of multi-datasource searching,
// pagination, date filtering, and result aggregation into a simple, reusable API.
package search

import (
	"strconv"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
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
	storageManager *storage.Manager
}

// NewSearchService creates a new search service with the provided storage manager.
// The storage manager is used to execute searches across all configured datasources.
//
// Parameters:
//   - storageManager: The storage manager that provides access to all datasource databases
//
// Returns:
//   - *SearchService: A new search service instance ready to execute searches
func NewSearchService(storageManager *storage.Manager) *SearchService {
	return &SearchService{
		storageManager: storageManager,
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
	results, totalResults, hasMoreResults, totalPages := getSearchResultsWithDateRange(
		s.storageManager,
		params.Query,
		params.DatasourceFilters,
		params.Page,
		params.Limit,
		params.StartDate,
		params.EndDate,
	)

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

// getSearchResultsWithDateRange executes the actual search operation with date filtering.
// This is an internal function that interfaces with the storage manager to perform
// multi-datasource searches with pagination and date range filtering.
//
// The function handles:
// - Datasource filtering (specific datasources vs all datasources)
// - Date range filtering (with or without date constraints)
// - Pagination with lookahead for "has more" detection
// - Result counting and pagination metadata calculation
//
// Parameters:
//   - storageManager: Storage manager for database access
//   - query: Search term to look for
//   - datasourceFilters: List of datasource names to search (empty = all)
//   - page: Current page number (1-based)
//   - totalLimit: Maximum results per page
//   - startDate: Optional start date filter
//   - endDate: Optional end date filter
//
// Returns:
//   - map[string][]core.Block: Results grouped by datasource name
//   - int: Total count of results on this page
//   - bool: Whether more results are available
//   - int: Estimated total number of pages
func getSearchResultsWithDateRange(storageManager *storage.Manager, query string, datasourceFilters []string, page, totalLimit int, startDate, endDate *time.Time) (map[string][]core.Block, int, bool, int) {
	var results map[string][]core.Block
	var err error

	if len(datasourceFilters) > 0 {
		if startDate != nil || endDate != nil {
			results, err = storageManager.SearchDatasourcesPagedWithDateRange(datasourceFilters, query, totalLimit*page*2, page, totalLimit, startDate, endDate)
		} else {
			results, err = storageManager.SearchDatasourcesPaged(datasourceFilters, query, totalLimit*page*2, page, totalLimit)
		}
	} else {
		if startDate != nil || endDate != nil {
			results, err = storageManager.SearchAllDatasourcesPagedWithDateRange(query, totalLimit*page*2, page, totalLimit, startDate, endDate)
		} else {
			results, err = storageManager.SearchAllDatasourcesPaged(query, totalLimit*page*2, page, totalLimit)
		}
	}
	if err != nil {
		return make(map[string][]core.Block), 0, false, 1
	}

	totalResults := 0
	for _, blocks := range results {
		totalResults += len(blocks)
	}

	if totalResults == 0 && page > 1 {
		return make(map[string][]core.Block), 0, false, page
	}

	hasMoreResults := false
	if totalResults == totalLimit {
		var nextPageResults map[string][]core.Block
		if len(datasourceFilters) > 0 {
			if startDate != nil || endDate != nil {
				nextPageResults, err = storageManager.SearchDatasourcesPagedWithDateRange(datasourceFilters, query, totalLimit*(page+1)*2, page+1, 1, startDate, endDate)
			} else {
				nextPageResults, err = storageManager.SearchDatasourcesPaged(datasourceFilters, query, totalLimit*(page+1)*2, page+1, 1)
			}
		} else {
			if startDate != nil || endDate != nil {
				nextPageResults, err = storageManager.SearchAllDatasourcesPagedWithDateRange(query, totalLimit*(page+1)*2, page+1, 1, startDate, endDate)
			} else {
				nextPageResults, err = storageManager.SearchAllDatasourcesPaged(query, totalLimit*(page+1)*2, page+1, 1)
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

	totalPages := page
	if hasMoreResults {
		totalPages = page + 1
	}

	if totalResults == 0 && page > 1 {
		totalPages = page
	}

	return results, totalResults, hasMoreResults, totalPages
}
