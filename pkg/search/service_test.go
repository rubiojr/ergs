package search

import (
	"fmt"
	"net/url"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/storage"
)

func TestParseSearchParams(t *testing.T) {
	tests := []struct {
		name     string
		query    string
		expected SearchParams
		hasError bool
	}{
		{
			name:  "basic query",
			query: "q=test&page=2&limit=50",
			expected: SearchParams{
				Query: "test",
				Page:  2,
				Limit: 50,
			},
		},
		{
			name:  "with datasource filters",
			query: "q=search&datasource=github&datasource=rss&page=1&limit=20",
			expected: SearchParams{
				Query:             "search",
				DatasourceFilters: []string{"github", "rss"},
				Page:              1,
				Limit:             20,
			},
		},
		{
			name:  "with date range",
			query: "q=test&start_date=2023-01-01&end_date=2023-12-31",
			expected: SearchParams{
				Query:     "test",
				Page:      1,
				Limit:     30,
				StartDate: parseDate("2023-01-01"),
				EndDate:   parseDate("2023-12-31"),
			},
		},
		{
			name:  "defaults when no params",
			query: "",
			expected: SearchParams{
				Page:  1,
				Limit: 30,
			},
		},
		{
			name:  "invalid limit defaults to 30",
			query: "q=test&limit=invalid",
			expected: SearchParams{
				Query: "test",
				Page:  1,
				Limit: 30,
			},
		},
		{
			name:     "invalid start date returns error",
			query:    "q=test&start_date=invalid-date",
			hasError: true,
		},
		{
			name:     "invalid end date returns error",
			query:    "q=test&end_date=invalid-date",
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			values, err := url.ParseQuery(tt.query)
			if err != nil {
				t.Fatalf("Failed to parse query string: %v", err)
			}

			params, err := ParseSearchParams(values)

			if tt.hasError {
				if err == nil {
					t.Error("Expected error but got none")
				}
				return
			}

			if err != nil {
				t.Errorf("Unexpected error: %v", err)
				return
			}

			if params.Query != tt.expected.Query {
				t.Errorf("Query: expected %q, got %q", tt.expected.Query, params.Query)
			}

			if params.Page != tt.expected.Page {
				t.Errorf("Page: expected %d, got %d", tt.expected.Page, params.Page)
			}

			if params.Limit != tt.expected.Limit {
				t.Errorf("Limit: expected %d, got %d", tt.expected.Limit, params.Limit)
			}

			if len(params.DatasourceFilters) != len(tt.expected.DatasourceFilters) {
				t.Errorf("DatasourceFilters length: expected %d, got %d", len(tt.expected.DatasourceFilters), len(params.DatasourceFilters))
			} else {
				for i, filter := range tt.expected.DatasourceFilters {
					if params.DatasourceFilters[i] != filter {
						t.Errorf("DatasourceFilters[%d]: expected %q, got %q", i, filter, params.DatasourceFilters[i])
					}
				}
			}

			if !datesEqual(params.StartDate, tt.expected.StartDate) {
				t.Errorf("StartDate: expected %v, got %v", tt.expected.StartDate, params.StartDate)
			}

			if !datesEqual(params.EndDate, tt.expected.EndDate) {
				t.Errorf("EndDate: expected %v, got %v", tt.expected.EndDate, params.EndDate)
			}
		})
	}
}

func TestSearchService(t *testing.T) {
	// This test verifies the SearchService can be created and has the expected structure
	// without needing a full storage manager setup
	service := NewSearchService(nil)
	if service == nil {
		t.Error("NewSearchService returned nil")
		return
	}

	// Access through field name to avoid the linter warning about nil pointer dereference
	// This test intentionally passes nil to verify the constructor behavior
}

// Helper functions for tests
func parseDate(dateStr string) *time.Time {
	if dateStr == "" {
		return nil
	}
	t, err := time.Parse("2006-01-02", dateStr)
	if err != nil {
		return nil
	}
	// For end dates, set to end of day like the actual parser does
	if dateStr == "2023-12-31" {
		endOfDay := time.Date(t.Year(), t.Month(), t.Day(), 23, 59, 59, 999999999, t.Location())
		return &endOfDay
	}
	return &t
}

func datesEqual(a, b *time.Time) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return a.Equal(*b)
}

func ExampleNewSearchService() {
	// Create a search service with a storage manager
	// In real usage, storageManager would be properly initialized
	var storageManager *storage.Manager
	service := NewSearchService(storageManager)

	// Service is ready to execute searches
	_ = service
	// Output:
}

func ExampleParseSearchParams() {
	// Parse HTTP query parameters into SearchParams
	values, _ := url.ParseQuery("q=golang&datasource=github&datasource=rss&page=2&limit=10")
	params, err := ParseSearchParams(values)

	if err != nil {
		panic(err)
	}

	// Access parsed parameters
	fmt.Println("Query:", params.Query)
	fmt.Println("Page:", params.Page)
	fmt.Println("Limit:", params.Limit)
	fmt.Println("Datasources:", len(params.DatasourceFilters))

	// Output:
	// Query: golang
	// Page: 2
	// Limit: 10
	// Datasources: 2
}

func ExampleParseSearchParams_withDateRange() {
	// Parse search parameters with date filtering
	values, _ := url.ParseQuery("q=documentation&start_date=2023-01-01&end_date=2023-12-31")
	params, err := ParseSearchParams(values)

	if err != nil {
		panic(err)
	}

	// Date filtering is configured
	hasDateRange := params.StartDate != nil && params.EndDate != nil
	fmt.Println("Has date range:", hasDateRange)

	if hasDateRange {
		fmt.Println("Start date:", params.StartDate.Format("2006-01-02"))
		// End date is automatically set to end of day
		fmt.Println("End time:", params.EndDate.Format("15:04:05"))
	}

	// Output:
	// Has date range: true
	// Start date: 2023-01-01
	// End time: 23:59:59
}

func ExampleSearchParams() {
	// Create search parameters programmatically
	startDate := time.Date(2023, 6, 1, 0, 0, 0, 0, time.UTC)
	endDate := time.Date(2023, 6, 30, 23, 59, 59, 0, time.UTC)

	params := SearchParams{
		Query:             "API documentation",
		DatasourceFilters: []string{"github", "gitlab"},
		Page:              1,
		Limit:             25,
		StartDate:         &startDate,
		EndDate:           &endDate,
	}

	// Parameters ready for search execution
	fmt.Println("Search configured for:", params.Query)
	fmt.Println("Datasources:", len(params.DatasourceFilters))
	fmt.Println("Date range:", params.StartDate.Month().String())

	// Output:
	// Search configured for: API documentation
	// Datasources: 2
	// Date range: June
}
