// Package search provides a unified search interface for the Ergs data indexing system.
//
// # Overview
//
// This package abstracts the complexity of searching across multiple datasources,
// handling parameter parsing, pagination, date filtering, and result aggregation.
// It serves as the core search engine for both the REST API and web interface.
//
// # Key Features
//
//   - Multi-datasource search with optional filtering
//   - Flexible pagination with lookahead detection
//   - Date range filtering with automatic end-of-day handling
//   - Parameter parsing from HTTP query strings
//   - Result aggregation and metadata calculation
//   - Clean separation between search logic and presentation
//
// # Architecture
//
// The search package is designed around two main components:
//
//   - SearchService: Executes searches using the storage manager
//   - Parameter parsing: Converts HTTP parameters to structured SearchParams
//
// This design allows the same search logic to be used by different interfaces
// (API, web UI) while maintaining consistent behavior and avoiding code duplication.
//
// # Usage Examples
//
// Basic search across all datasources:
//
//	service := search.NewSearchService(storageManager)
//	params := search.SearchParams{
//		Query: "golang programming",
//		Page:  1,
//		Limit: 20,
//	}
//	results, err := service.Search(params)
//
// Search with datasource filtering:
//
//	params := search.SearchParams{
//		Query:             "API documentation",
//		DatasourceFilters: []string{"github", "rss"},
//		Page:              1,
//		Limit:             10,
//	}
//	results, err := service.Search(params)
//
// Search with date range:
//
//	startDate := time.Date(2023, 1, 1, 0, 0, 0, 0, time.UTC)
//	endDate := time.Date(2023, 12, 31, 23, 59, 59, 0, time.UTC)
//	params := search.SearchParams{
//		Query:     "year in review",
//		StartDate: &startDate,
//		EndDate:   &endDate,
//		Page:      1,
//		Limit:     50,
//	}
//	results, err := service.Search(params)
//
// Parsing HTTP parameters:
//
//	// In an HTTP handler
//	params, err := search.ParseSearchParams(r.URL.Query())
//	if err != nil {
//		// Handle invalid date format
//		return
//	}
//	results, err := searchService.Search(params)
//
// # Search Behavior
//
// The search system operates across multiple isolated datasource databases,
// each containing blocks of indexed data. Key behavioral characteristics:
//
//   - Results are grouped by datasource for efficient display
//   - Pagination uses estimated totals due to distributed nature
//   - Date filtering applies to block creation timestamps
//   - Empty queries are allowed (useful for browse-only interfaces)
//   - Search terms support full SQLite FTS5 syntax
//
// # Performance Considerations
//
// The search system is designed for efficiency across large datasets:
//
//   - Parallel execution across multiple datasource databases
//   - Pagination prevents loading entire result sets
//   - Lookahead queries for accurate "has more" detection
//   - Conservative memory usage through streaming results
//
// # Integration
//
// This package integrates with:
//
//   - pkg/storage: For multi-datasource database access
//   - pkg/core: For block interfaces and data structures
//   - pkg/api: As the search backend for REST endpoints
//   - cmd/web: As the search backend for HTML interfaces
//
// The clean interface design allows easy testing and future extensibility
// while maintaining the existing behavior that users and applications depend on.
package search
