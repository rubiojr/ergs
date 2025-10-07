package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

type mockBlock struct {
	id        string
	text      string
	createdAt time.Time
	source    string
	metadata  map[string]interface{}
}

func (b *mockBlock) ID() string                       { return b.id }
func (b *mockBlock) Text() string                     { return b.text }
func (b *mockBlock) CreatedAt() time.Time             { return b.createdAt }
func (b *mockBlock) Source() string                   { return b.source }
func (b *mockBlock) Type() string                     { return "test" }
func (b *mockBlock) Metadata() map[string]interface{} { return b.metadata }
func (b *mockBlock) PrettyText() string               { return b.text }
func (b *mockBlock) Summary() string                  { return b.text }
func (b *mockBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
	return &mockBlock{
		id:        genericBlock.ID(),
		text:      genericBlock.Text(),
		createdAt: genericBlock.CreatedAt(),
		source:    source,
		metadata:  genericBlock.Metadata(),
	}
}

type mockDatasource struct {
	name string
}

func (d *mockDatasource) Name() string { return d.name }
func (d *mockDatasource) Type() string { return "test" }
func (d *mockDatasource) Close() error { return nil }
func (d *mockDatasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	return nil
}
func (d *mockDatasource) Schema() map[string]any {
	return map[string]any{
		"text":       "TEXT",
		"created_at": "DATETIME",
		"metadata":   "TEXT",
	}
}
func (d *mockDatasource) BlockPrototype() core.Block         { return &mockBlock{} }
func (d *mockDatasource) ConfigType() interface{}            { return nil }
func (d *mockDatasource) SetConfig(config interface{}) error { return nil }
func (d *mockDatasource) GetConfig() interface{}             { return nil }
func (d *mockDatasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return &mockDatasource{name: instanceName}, nil
}

func setupTestAPIServer(t *testing.T) (*http.ServeMux, func()) {
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
	registry := core.NewRegistry()

	// Create test data
	testData := map[string][]core.Block{
		"datasource1": {
			&mockBlock{
				id:        "1",
				text:      "Block 1 from datasource1",
				createdAt: time.Date(2023, 1, 1, 12, 0, 0, 0, time.UTC),
				source:    "test1",
				metadata:  map[string]interface{}{"test": "value1"},
			},
			&mockBlock{
				id:        "2",
				text:      "Block 2 from datasource1",
				createdAt: time.Date(2023, 1, 2, 12, 0, 0, 0, time.UTC),
				source:    "test2",
				metadata:  map[string]interface{}{"test": "value2"},
			},
		},
		"datasource2": {
			&mockBlock{
				id:        "3",
				text:      "Block 3 from datasource2",
				createdAt: time.Date(2023, 1, 3, 12, 0, 0, 0, time.UTC),
				source:    "test3",
				metadata:  map[string]interface{}{"test": "value3"},
			},
		},
	}

	// Setup storage with test data
	for datasourceName, blocks := range testData {
		schema := map[string]any{
			"text":       "TEXT",
			"created_at": "DATETIME",
			"metadata":   "TEXT",
		}

		err := storageManager.InitializeDatasourceStorage(datasourceName, schema)
		if err != nil {
			t.Fatalf("Failed to initialize storage for %s: %v", datasourceName, err)
		}

		storage, err := storageManager.EnsureStorageWithMigrations(datasourceName)
		if err != nil {
			t.Fatalf("Failed to get storage for %s: %v", datasourceName, err)
		}

		for _, block := range blocks {
			err = storage.StoreBlock(block, "test")
			if err != nil {
				t.Fatalf("Failed to store block in %s: %v", datasourceName, err)
			}
		}

		mockBlockPrototype := &mockBlock{}
		storageManager.RegisterBlockPrototype(datasourceName, mockBlockPrototype)
	}

	// Create mock datasources and add to registry
	mockPrototype := &mockDatasource{}
	err := registry.RegisterPrototype("test", mockPrototype)
	if err != nil {
		t.Fatalf("Failed to register prototype: %v", err)
	}

	err = registry.CreateDatasource("datasource1", "test", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource1: %v", err)
	}

	err = registry.CreateDatasource("datasource2", "test", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource2: %v", err)
	}

	server := NewServer(registry, storageManager)
	mux := http.NewServeMux()
	server.RegisterRoutes(mux)

	cleanup := func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
		if err := registry.Close(); err != nil {
			t.Errorf("Failed to close registry: %v", err)
		}
	}

	return mux, cleanup
}

func TestAPIListDatasources(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/datasources", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestAPISearch(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/search?q=Block", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestAPISearchMissingQuery(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestAPIDatasourceBlocks(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/datasources/datasource1", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestAPIDatasourceBlocksNotFound(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/datasources/nonexistent", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected status 404, got %d", w.Code)
	}
}

func TestAPIStats(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/stats", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestAPIHealth(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	if contentType := w.Header().Get("Content-Type"); contentType != "application/json" {
		t.Errorf("Expected Content-Type application/json, got %s", contentType)
	}
}

func TestAPISearchErrorHandling(t *testing.T) {
	// Test that API search handles FTS5 syntax errors gracefully
	tempDir, err := os.MkdirTemp("", "ergs-api-error-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir: %v", err)
		}
	}()

	registry := core.NewRegistry()
	storageManager, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create storage manager: %v", err)
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	// Create test datasource and add some data to trigger FTS5 errors
	testStorage, err := storageManager.EnsureStorageWithMigrations("test-datasource")
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	// Add test data so FTS5 queries actually run
	testBlock := core.NewGenericBlock("1", "test content", "source1", "test", time.Now(), nil)
	err = testStorage.StoreBlock(testBlock, "test")
	if err != nil {
		t.Fatalf("Failed to store test block: %v", err)
	}

	// Create API server
	server := NewServer(registry, storageManager)

	// Test cases with various FTS5 syntax errors
	testCases := []struct {
		name           string
		query          string
		expectedStatus int
		expectedType   string
		shouldContain  string
	}{
		{
			name:           "forward_slash_error",
			query:          "KG7x/Quake3e",
			expectedStatus: http.StatusBadRequest,
			expectedType:   "Invalid search query",
			shouldContain:  "Forward slashes (/) are not allowed",
		},
		{
			name:           "unmatched_quote_error",
			query:          "test 'unmatched",
			expectedStatus: http.StatusBadRequest,
			expectedType:   "Invalid search query",
			shouldContain:  "Unmatched single quotes detected",
		},
		{
			name:           "general_syntax_error",
			query:          "test & invalid",
			expectedStatus: http.StatusBadRequest,
			expectedType:   "Invalid search query",
			shouldContain:  "Invalid search syntax",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a request with the problematic query
			req, err := http.NewRequest("GET", "/api/search?q="+url.QueryEscape(tc.query), nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Handle the request
			server.HandleSearch(rr, req)

			// Should return 400 Bad Request (not 500) for syntax errors
			if rr.Code != tc.expectedStatus {
				t.Errorf("Expected status %d, got %d", tc.expectedStatus, rr.Code)
			}

			// Parse response
			var response map[string]interface{}
			err = json.Unmarshal(rr.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			// Check error type
			if errorType, ok := response["error"].(string); !ok || errorType != tc.expectedType {
				t.Errorf("Expected error type %q, got %q", tc.expectedType, errorType)
			}

			// Check error message content
			if message, ok := response["message"].(string); !ok || !strings.Contains(message, tc.shouldContain) {
				t.Errorf("Expected message to contain %q, got %q", tc.shouldContain, message)
			}

			// Should not contain raw SQLite error messages
			if message, ok := response["message"].(string); ok && strings.Contains(message, "sqlite3: SQL logic error") {
				t.Error("Response should not contain raw SQLite error messages")
			}
		})
	}
}

func TestFormatAPISearchError(t *testing.T) {
	// Test the formatAPISearchError function directly
	testCases := []struct {
		name     string
		input    error
		expected string
	}{
		{
			name:     "forward_slash_syntax_error",
			input:    fmt.Errorf("searching hackernews: sqlite3: SQL logic error: fts5: syntax error near \"/\""),
			expected: "Forward slashes (/) are not allowed in search terms",
		},
		{
			name:     "single_quote_syntax_error",
			input:    fmt.Errorf("searching test: sqlite3: SQL logic error: fts5: syntax error near \"'\""),
			expected: "Unmatched single quotes detected. Use double quotes for phrase searches",
		},
		{
			name:     "general_syntax_error",
			input:    fmt.Errorf("searching test: sqlite3: SQL logic error: fts5: syntax error near \"&\""),
			expected: "Invalid search syntax. Check for special characters or invalid operators",
		},
		{
			name:     "sql_logic_error",
			input:    fmt.Errorf("searching test: sqlite3: SQL logic error: some other issue"),
			expected: "Search query contains invalid syntax",
		},
		{
			name:     "unknown_error",
			input:    fmt.Errorf("completely unknown error"),
			expected: "Invalid search query format",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatAPISearchError(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestIsFTS5SyntaxError(t *testing.T) {
	// Test the isFTS5SyntaxError function
	testCases := []struct {
		name     string
		input    error
		expected bool
	}{
		{
			name:     "fts5_syntax_error",
			input:    fmt.Errorf("sqlite3: SQL logic error: fts5: syntax error near \"/\""),
			expected: true,
		},
		{
			name:     "sql_logic_error",
			input:    fmt.Errorf("sqlite3: SQL logic error: something"),
			expected: true,
		},
		{
			name:     "other_error",
			input:    fmt.Errorf("network timeout"),
			expected: false,
		},
		{
			name:     "database_locked",
			input:    fmt.Errorf("database is locked"),
			expected: false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := isFTS5SyntaxError(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %v, got %v", tc.expected, result)
			}
		})
	}
}

func TestAPIMethodNotAllowed(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	testCases := []struct {
		method   string
		endpoint string
	}{
		{"POST", "/api/datasources"},
		{"PUT", "/api/datasources"},
		{"DELETE", "/api/datasources"},
		{"POST", "/api/search"},
		{"PUT", "/api/search"},
		{"DELETE", "/api/search"},
		{"POST", "/api/stats"},
		{"PUT", "/api/stats"},
		{"DELETE", "/api/stats"},
		{"POST", "/health"},
		{"PUT", "/health"},
		{"DELETE", "/health"},
		{"POST", "/api/datasources/datasource1"},
		{"PUT", "/api/datasources/datasource1"},
		{"DELETE", "/api/datasources/datasource1"},
	}

	for _, tc := range testCases {
		t.Run(tc.method+"_"+tc.endpoint, func(t *testing.T) {
			req := httptest.NewRequest(tc.method, tc.endpoint, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusMethodNotAllowed {
				t.Errorf("Expected status 405 for %s %s, got %d", tc.method, tc.endpoint, w.Code)
			}
		})
	}
}

func TestAPIInvalidPaths(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	testCases := []struct {
		path           string
		expectedStatus int
	}{
		{"/api/nonexistent", http.StatusNotFound},
		{"/api/datasources/", http.StatusNotFound}, // Empty name parameter
		{"/nonexistent", http.StatusNotFound},
	}

	for _, tc := range testCases {
		t.Run(tc.path, func(t *testing.T) {
			req := httptest.NewRequest("GET", tc.path, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != tc.expectedStatus {
				t.Errorf("Expected status %d for %s, got %d", tc.expectedStatus, tc.path, w.Code)
			}
		})
	}
}

func TestAPIDatasourceBlocksDateFiltering(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	testCases := []struct {
		name          string
		queryParams   string
		expectedCount int
		description   string
	}{
		{
			name:          "filter_by_start_date_only",
			queryParams:   "start_date=2023-01-02",
			expectedCount: 1, // Only block 2 from datasource1 (2023-01-02)
			description:   "Should return blocks on or after 2023-01-02",
		},
		{
			name:          "filter_by_end_date_only",
			queryParams:   "end_date=2023-01-01",
			expectedCount: 1, // Only block 1 from datasource1 (2023-01-01)
			description:   "Should return blocks on or before 2023-01-01",
		},
		{
			name:          "filter_by_date_range",
			queryParams:   "start_date=2023-01-01&end_date=2023-01-02",
			expectedCount: 2, // Blocks 1 and 2 from datasource1
			description:   "Should return blocks within date range",
		},
		{
			name:          "filter_no_matches",
			queryParams:   "start_date=2024-01-01",
			expectedCount: 0, // No blocks in 2024
			description:   "Should return no blocks when date range has no matches",
		},
		{
			name:          "filter_exact_date",
			queryParams:   "start_date=2023-01-01&end_date=2023-01-01",
			expectedCount: 1, // Only block 1 (end_date includes end of day)
			description:   "Should return blocks on exact date",
		},
		{
			name:          "no_date_filter",
			queryParams:   "",
			expectedCount: 2, // All blocks from datasource1
			description:   "Should return all blocks when no date filter",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqURL := "/api/datasources/datasource1"
			if tc.queryParams != "" {
				reqURL += "?" + tc.queryParams
			}

			req := httptest.NewRequest("GET", reqURL, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
			}

			var response ListBlocksResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response.Count != tc.expectedCount {
				t.Errorf("%s: Expected %d blocks, got %d", tc.description, tc.expectedCount, response.Count)
				t.Logf("Blocks returned: %+v", response.Blocks)
			}

			// Verify all returned blocks are within the date range if specified
			if tc.queryParams != "" {
				params, _ := url.ParseQuery(tc.queryParams)
				if startDateStr := params["start_date"]; len(startDateStr) > 0 && startDateStr[0] != "" {
					startDate, _ := time.Parse("2006-01-02", startDateStr[0])
					for _, block := range response.Blocks {
						if block.CreatedAt.Before(startDate) {
							t.Errorf("Block %s created at %v is before start_date %v", block.ID, block.CreatedAt, startDate)
						}
					}
				}
				if endDateStr := params["end_date"]; len(endDateStr) > 0 && endDateStr[0] != "" {
					endDate, _ := time.Parse("2006-01-02", endDateStr[0])
					endOfDay := time.Date(endDate.Year(), endDate.Month(), endDate.Day(), 23, 59, 59, 999999999, endDate.Location())
					for _, block := range response.Blocks {
						if block.CreatedAt.After(endOfDay) {
							t.Errorf("Block %s created at %v is after end_date %v", block.ID, block.CreatedAt, endOfDay)
						}
					}
				}
			}
		})
	}
}

func TestAPISearchDateFiltering(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	testCases := []struct {
		name          string
		queryParams   string
		expectedTotal int
		description   string
	}{
		{
			name:          "search_with_start_date",
			queryParams:   "q=Block&start_date=2023-01-03",
			expectedTotal: 1, // Only block 3 from datasource2
			description:   "Should return blocks matching query and after start_date",
		},
		{
			name:          "search_with_end_date",
			queryParams:   "q=Block&end_date=2023-01-01",
			expectedTotal: 1, // Only block 1 from datasource1
			description:   "Should return blocks matching query and before end_date",
		},
		{
			name:          "search_with_date_range",
			queryParams:   "q=Block&start_date=2023-01-02&end_date=2023-01-03",
			expectedTotal: 2, // Blocks 2 and 3
			description:   "Should return blocks matching query within date range",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqURL := "/api/search?" + tc.queryParams

			req := httptest.NewRequest("GET", reqURL, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Fatalf("Expected status 200, got %d. Body: %s", w.Code, w.Body.String())
			}

			var response SearchResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			if response.TotalCount != tc.expectedTotal {
				t.Errorf("%s: Expected %d total blocks, got %d", tc.description, tc.expectedTotal, response.TotalCount)
			}
		})
	}
}

func TestAPIInvalidDateFormats(t *testing.T) {
	mux, cleanup := setupTestAPIServer(t)
	defer cleanup()

	testCases := []struct {
		name        string
		endpoint    string
		queryParams string
		description string
	}{
		{
			name:        "invalid_start_date_datasource",
			endpoint:    "/api/datasources/datasource1",
			queryParams: "start_date=invalid-date",
			description: "Should return error for invalid start_date format in datasource endpoint",
		},
		{
			name:        "invalid_end_date_datasource",
			endpoint:    "/api/datasources/datasource1",
			queryParams: "end_date=2023/01/01",
			description: "Should return error for invalid end_date format in datasource endpoint",
		},
		{
			name:        "invalid_start_date_search",
			endpoint:    "/api/search",
			queryParams: "q=test&start_date=01-01-2023",
			description: "Should return error for invalid start_date format in search endpoint",
		},
		{
			name:        "invalid_end_date_search",
			endpoint:    "/api/search",
			queryParams: "q=test&end_date=not-a-date",
			description: "Should return error for invalid end_date format in search endpoint",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			reqURL := tc.endpoint + "?" + tc.queryParams

			req := httptest.NewRequest("GET", reqURL, nil)
			w := httptest.NewRecorder()

			mux.ServeHTTP(w, req)

			if w.Code != http.StatusBadRequest {
				t.Errorf("%s: Expected status 400, got %d", tc.description, w.Code)
			}

			var response ErrorResponse
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to parse error response: %v", err)
			}

			if !strings.Contains(response.Error, "Invalid date format") {
				t.Errorf("%s: Expected 'Invalid date format' error, got %s", tc.description, response.Error)
			}
		})
	}
}
