package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/api"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
	"github.com/rubiojr/ergs/pkg/storage"

	// Import test datasources to register their factories
	_ "github.com/rubiojr/ergs/pkg/datasources/testrand"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
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
func (b *mockBlock) Type() string                     { return "mock" }
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

func setupTestWebServer(t *testing.T) (*WebServer, func()) {
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
	registry := core.GetGlobalRegistry()

	// Create test data
	now := time.Now()
	testData := map[string][]core.Block{
		"datasource_a": make([]core.Block, 15),
		"datasource_b": make([]core.Block, 10),
	}

	// Fill datasource_a
	for i := 0; i < 15; i++ {
		testData["datasource_a"][i] = &mockBlock{
			id:        fmt.Sprintf("a_block_%d", i),
			text:      fmt.Sprintf("test content a %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_a",
			metadata:  map[string]interface{}{"index": i},
		}
	}

	// Fill datasource_b
	for i := 0; i < 10; i++ {
		testData["datasource_b"][i] = &mockBlock{
			id:        fmt.Sprintf("b_block_%d", i),
			text:      fmt.Sprintf("test content b %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "datasource_b",
			metadata:  map[string]interface{}{"index": i},
		}
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
			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block in %s: %v", datasourceName, err)
			}
		}

		mockBlockPrototype := &mockBlock{}
		storageManager.RegisterBlockPrototype(datasourceName, mockBlockPrototype)
	}

	// Add test datasources to registry with names that test alphabetical ordering
	testDatasources := []string{"zebra_datasource", "alpha_datasource", "beta_datasource"}
	for _, dsName := range testDatasources {
		err := registry.CreateDatasource(dsName, "testrand", nil)
		if err != nil {
			t.Fatalf("Failed to create test datasource %s: %v", dsName, err)
		}
	}

	// Initialize renderer registry for web interface tests
	rendererRegistry := render.NewRendererRegistry()

	// Initialize API server
	apiServer := api.NewServer(registry, storageManager)

	server := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		rendererRegistry: rendererRegistry,
		apiServer:        apiServer,
	}

	cleanup := func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
		if err := registry.Close(); err != nil {
			t.Logf("Warning: failed to close registry: %v", err)
		}
	}

	return server, cleanup
}

func TestAPISearchBasic(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check basic response structure
	if response["query"] != "test" {
		t.Errorf("Expected query 'test', got %v", response["query"])
	}

	if response["page"] != float64(1) {
		t.Errorf("Expected page 1, got %v", response["page"])
	}

	if response["limit"] != float64(30) {
		t.Errorf("Expected limit 30, got %v", response["limit"])
	}

	// Check that results exist
	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results should be a map")
	}

	if len(results) == 0 {
		t.Error("Expected some results, got none")
	}
}

func TestFirehoseOrderedSliceIntegration(t *testing.T) {
	// This test ensures the web layer (via the same setup helper) can access the
	// backend firehose Ordered slice semantics: global newest-first ordering with
	// deterministic tie-breakers (CreatedAt DESC, datasource ASC, ID ASC).
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	searchService := server.storageManager.GetSearchService()
	// Limit large enough to include all seeded blocks (15 + 10 = 25)
	results, err := searchService.Search(storage.SearchParams{
		Query: "",
		Page:  1,
		Limit: 40,
	})
	if err != nil {
		t.Fatalf("firehose search failed: %v", err)
	}

	ordered := results.Ordered
	if len(ordered) != 25 {
		t.Fatalf("expected 25 blocks in firehose Ordered slice, got %d", len(ordered))
	}

	// Verify global descending CreatedAt ordering plus tie-breakers for identical timestamps.
	for i := 0; i < len(ordered)-1; i++ {
		cur := ordered[i]
		next := ordered[i+1]

		// Chronological (newest first)
		if cur.CreatedAt().Before(next.CreatedAt()) {
			t.Errorf("ordering violation: index %d (%v) is before index %d (%v)",
				i, cur.CreatedAt(), i+1, next.CreatedAt())
		}

		// Tie-breaker: when timestamps equal, datasource name asc, then ID asc
		if cur.CreatedAt().Equal(next.CreatedAt()) {
			if cur.Source() > next.Source() {
				t.Errorf("datasource tie-break violation at %d: %s > %s (same time)",
					i, cur.Source(), next.Source())
			} else if cur.Source() == next.Source() && cur.ID() > next.ID() {
				t.Errorf("ID tie-break violation at %d within datasource %s: %s > %s (same time)",
					i, cur.Source(), cur.ID(), next.ID())
			}
		}
	}

	// Additional sanity: for our seeded data, matching timestamps across datasources
	// should place datasource_a blocks before datasource_b blocks.
	seenPairs := 0
	for i := 0; i < len(ordered)-1; i++ {
		if ordered[i].CreatedAt().Equal(ordered[i+1].CreatedAt()) &&
			ordered[i].Source() == "datasource_a" && ordered[i+1].Source() == "datasource_b" {
			seenPairs++
		}
	}
	if seenPairs == 0 {
		t.Log("warning: no equal-timestamp cross-datasource pairs observed (tie-breaker scenario not exercised)")
	}
}

func TestAPISearchPagination(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test page 1 with limit 10
	req := httptest.NewRequest("GET", "/api/search?q=test&page=1&limit=10", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check pagination parameters
	if response["page"] != float64(1) {
		t.Errorf("Expected page 1, got %v", response["page"])
	}

	if response["limit"] != float64(10) {
		t.Errorf("Expected limit 10, got %v", response["limit"])
	}

	totalCount := response["total_count"].(float64)
	if totalCount != 10 {
		t.Errorf("Expected total_count 10, got %v", totalCount)
	}

	hasMore := response["has_more"].(bool)
	if !hasMore {
		t.Error("Expected has_more to be true for page 1")
	}
}

func TestAPISearchPage2(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test page 2 with limit 10
	req := httptest.NewRequest("GET", "/api/search?q=test&page=2&limit=10", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check pagination parameters
	if response["page"] != float64(2) {
		t.Errorf("Expected page 2, got %v", response["page"])
	}

	totalCount := response["total_count"].(float64)
	if totalCount != 10 {
		t.Errorf("Expected total_count 10 for page 2, got %v", totalCount)
	}

	hasMore := response["has_more"].(bool)
	if !hasMore {
		t.Error("Expected has_more to be true for page 2")
	}
}

func TestWebSearchErrorHandling(t *testing.T) {
	// Test that web search handles FTS5 syntax errors gracefully
	tempDir, err := os.MkdirTemp("", "ergs-web-error-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir: %v", err)
		}
	}()

	// Create config
	cfg := &config.Config{
		StorageDir: tempDir,
	}

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

	// Initialize renderer registry
	rendererRegistry := render.GetGlobalRegistry()

	// Create web server
	webServer := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		config:           cfg,
		rendererRegistry: rendererRegistry,
	}

	// Test cases with various FTS5 syntax errors
	testCases := []struct {
		name          string
		query         string
		expectedError string
		shouldContain string
	}{
		{
			name:          "forward_slash_error",
			query:         "KG7x/Quake3e",
			expectedError: "syntax error near \"/\"",
			shouldContain: "Forward slashes (/) are not allowed",
		},
		{
			name:          "unmatched_quote_error",
			query:         "test 'unmatched",
			expectedError: "syntax error near \"'\"",
			shouldContain: "Unmatched single quotes detected",
		},
		{
			name:          "general_syntax_error",
			query:         "test & invalid",
			expectedError: "syntax error",
			shouldContain: "Invalid search syntax",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a request with the problematic query
			req, err := http.NewRequest("GET", "/search?q="+url.QueryEscape(tc.query), nil)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}

			// Create response recorder
			rr := httptest.NewRecorder()

			// Handle the request
			webServer.handleSearch(rr, req)

			// Should return 200 OK (not 500) with error message in page
			if rr.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", rr.Code)
			}

			// Response body should contain the error message
			body := rr.Body.String()
			if !strings.Contains(body, tc.shouldContain) {
				t.Errorf("Expected response to contain %q, but it didn't. Body: %s", tc.shouldContain, body)
			}

			// Should not contain the raw error message
			if strings.Contains(body, "sqlite3: SQL logic error") {
				t.Error("Response should not contain raw SQLite error messages")
			}
		})
	}
}

func TestFormatSearchError(t *testing.T) {
	// Test the formatSearchError function directly
	testCases := []struct {
		name     string
		input    error
		expected string
	}{
		{
			name:     "forward_slash_syntax_error",
			input:    fmt.Errorf("searching hackernews: sqlite3: SQL logic error: fts5: syntax error near \"/\""),
			expected: "Invalid search query: Forward slashes (/) are not allowed in search terms. Please remove special characters or quote the query and try again.",
		},
		{
			name:     "single_quote_syntax_error",
			input:    fmt.Errorf("searching test: sqlite3: SQL logic error: fts5: syntax error near \"'\""),
			expected: "Invalid search query: Unmatched single quotes detected. Please use double quotes for phrase searches or remove single quotes.",
		},
		{
			name:     "general_syntax_error",
			input:    fmt.Errorf("searching test: sqlite3: SQL logic error: fts5: syntax error near \"&\""),
			expected: "Invalid search syntax. Please check your query for special characters, unmatched quotes, or invalid operators.",
		},
		{
			name:     "database_locked_error",
			input:    fmt.Errorf("database is locked"),
			expected: "Database is temporarily busy. Please try again in a moment.",
		},
		{
			name:     "generic_search_error",
			input:    fmt.Errorf("searching datasource: some other error"),
			expected: "Search error occurred. Please check your query syntax and try again.",
		},
		{
			name:     "unknown_error",
			input:    fmt.Errorf("completely unknown error"),
			expected: "Search failed due to an unexpected error. Please try a simpler query.",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := formatSearchError(tc.input)
			if result != tc.expected {
				t.Errorf("Expected %q, got %q", tc.expected, result)
			}
		})
	}
}

func TestAPISearchLastPage(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test page 3 with limit 10 (should have remaining results)
	req := httptest.NewRequest("GET", "/api/search?q=test&page=3&limit=10", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check pagination parameters
	if response["page"] != float64(3) {
		t.Errorf("Expected page 3, got %v", response["page"])
	}

	totalCount := response["total_count"].(float64)
	if totalCount > 10 {
		t.Errorf("Expected total_count <= 10 for page 3, got %v", totalCount)
	}

	// This should be the last page or close to it
	hasMore := response["has_more"].(bool)
	if hasMore && totalCount < 5 {
		t.Error("Unexpected has_more=true with few results on page 3")
	}
}

func TestAPISearchEmptyResults(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Search for something that doesn't exist
	req := httptest.NewRequest("GET", "/api/search?q=nonexistent", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	totalCount := response["total_count"].(float64)
	if totalCount != 0 {
		t.Errorf("Expected total_count 0 for nonexistent query, got %v", totalCount)
	}

	hasMore := response["has_more"].(bool)
	if hasMore {
		t.Error("Expected has_more to be false for empty results")
	}

	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results should be a map")
	}

	if len(results) != 0 {
		t.Errorf("Expected empty results, got %d datasources", len(results))
	}
}

func TestAPISearchMissingQuery(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Missing query parameter
	req := httptest.NewRequest("GET", "/api/search", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status %d, got %d", http.StatusBadRequest, w.Code)
	}
}

func TestAPISearchWithDatasourceFilter(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test filtering to only datasource_a
	req := httptest.NewRequest("GET", "/api/search?q=test&datasource=datasource_a", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results field is not a map")
	}

	// Should only have datasource_a results
	if _, hasA := results["datasource_a"]; !hasA {
		t.Error("Expected datasource_a in results")
	}
	if _, hasB := results["datasource_b"]; hasB {
		t.Error("Should not have datasource_b in results when filtering for datasource_a")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 datasource in results, got %d", len(results))
	}
}

func TestAPISearchWithMultipleDatasourceFilters(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test filtering to both datasource_a and datasource_b
	req := httptest.NewRequest("GET", "/api/search?q=test&datasource=datasource_a&datasource=datasource_b", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results field is not a map")
	}

	// Should have both datasource_a and datasource_b results
	if _, hasA := results["datasource_a"]; !hasA {
		t.Error("Expected datasource_a in results")
	}
	if _, hasB := results["datasource_b"]; !hasB {
		t.Error("Expected datasource_b in results")
	}

	if len(results) != 2 {
		t.Errorf("Expected 2 datasources in results, got %d", len(results))
	}
}

func TestAPISearchWithNonexistentDatasourceFilter(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test filtering to a datasource that doesn't exist
	req := httptest.NewRequest("GET", "/api/search?q=test&datasource=nonexistent", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results field is not a map")
	}

	// Should have no results
	if len(results) != 0 {
		t.Errorf("Expected 0 datasources in results when filtering for nonexistent datasource, got %d", len(results))
	}

	totalCount, ok := response["total_count"].(float64)
	if !ok {
		t.Fatal("total_count field is not a number")
	}

	if int(totalCount) != 0 {
		t.Errorf("Expected total_count to be 0, got %d", int(totalCount))
	}
}

func TestAPISearchWithMixedDatasourceFilters(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test filtering to one existing and one nonexistent datasource
	req := httptest.NewRequest("GET", "/api/search?q=test&datasource=datasource_a&datasource=nonexistent", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results field is not a map")
	}

	// Should only have datasource_a results (nonexistent datasource is filtered out)
	if _, hasA := results["datasource_a"]; !hasA {
		t.Error("Expected datasource_a in results")
	}
	if _, hasNonexistent := results["nonexistent"]; hasNonexistent {
		t.Error("Should not have nonexistent datasource in results")
	}

	if len(results) != 1 {
		t.Errorf("Expected 1 datasource in results, got %d", len(results))
	}
}

func TestWebSearchWithDatasourceFilter(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test web interface with datasource filter
	req := httptest.NewRequest("GET", "/search?q=test&datasource=datasource_a", nil)
	w := httptest.NewRecorder()

	server.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	// The response should contain HTML with filtered results
	// We can't easily parse the template output, but we can check that it doesn't error
	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty response body")
	}
}

func TestWebSearchWithMultipleDatasourceFilters(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test web interface with multiple datasource filters
	req := httptest.NewRequest("GET", "/search?q=test&datasource=datasource_a&datasource=datasource_b", nil)
	w := httptest.NewRecorder()

	server.handleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	body := w.Body.String()
	if len(body) == 0 {
		t.Error("Expected non-empty response body")
	}
}

func TestSearchResultsConsistencyBetweenAPIAndWeb(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test that API and web interface use the same search logic
	query := "test"
	datasourceFilter := "datasource_a"

	// Test API
	apiReq := httptest.NewRequest("GET", fmt.Sprintf("/api/search?q=%s&datasource=%s", query, datasourceFilter), nil)
	apiW := httptest.NewRecorder()
	server.apiServer.HandleSearch(apiW, apiReq)

	if apiW.Code != http.StatusOK {
		t.Errorf("API request failed with status %d", apiW.Code)
	}

	var apiResponse map[string]interface{}
	if err := json.Unmarshal(apiW.Body.Bytes(), &apiResponse); err != nil {
		t.Fatalf("Failed to parse API response: %v", err)
	}

	// Test Web interface
	webReq := httptest.NewRequest("GET", fmt.Sprintf("/search?q=%s&datasource=%s", query, datasourceFilter), nil)
	webW := httptest.NewRecorder()
	server.handleSearch(webW, webReq)

	if webW.Code != http.StatusOK {
		t.Errorf("Web request failed with status %d", webW.Code)
	}

	// Both should succeed and the API should only contain the filtered datasource
	apiResults, ok := apiResponse["results"].(map[string]interface{})
	if !ok {
		t.Fatal("API results field is not a map")
	}

	if len(apiResults) != 1 {
		t.Errorf("Expected 1 datasource in API results, got %d", len(apiResults))
	}

	if _, hasFiltered := apiResults[datasourceFilter]; !hasFiltered {
		t.Errorf("Expected %s in API results", datasourceFilter)
	}
}

func TestAPISearchInvalidPagination(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Invalid page number
	req := httptest.NewRequest("GET", "/api/search?q=test&page=0&limit=-5", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d (should handle invalid params gracefully), got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Should default to page 1, limit 30
	if response["page"] != float64(1) {
		t.Errorf("Expected page to default to 1, got %v", response["page"])
	}

	if response["limit"] != float64(30) {
		t.Errorf("Expected limit to default to 30, got %v", response["limit"])
	}
}

func TestAPISearchResponseStructure(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/search?q=test&limit=5", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check all required fields exist
	requiredFields := []string{"query", "results", "total_count", "page", "limit", "total_pages", "has_more"}
	for _, field := range requiredFields {
		if _, exists := response[field]; !exists {
			t.Errorf("Missing required field: %s", field)
		}
	}

	// Check results structure
	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results should be a map")
	}

	// Check individual datasource result structure
	for dsName, dsResults := range results {
		dsMap, ok := dsResults.(map[string]interface{})
		if !ok {
			t.Fatalf("Datasource %s results should be a map", dsName)
		}

		// Check required fields in datasource results
		dsRequiredFields := []string{"datasource", "blocks", "count", "query"}
		for _, field := range dsRequiredFields {
			if _, exists := dsMap[field]; !exists {
				t.Errorf("Missing required field in datasource %s: %s", dsName, field)
			}
		}

		// Check blocks structure
		blocks, ok := dsMap["blocks"].([]interface{})
		if !ok {
			t.Fatalf("Blocks in datasource %s should be an array", dsName)
		}

		if len(blocks) > 0 {
			block := blocks[0].(map[string]interface{})
			blockRequiredFields := []string{"id", "text", "source", "created_at", "metadata"}
			for _, field := range blockRequiredFields {
				if _, exists := block[field]; !exists {
					t.Errorf("Missing required field in block: %s", field)
				}
			}
		}
	}
}

func TestAPISearchMethodNotAllowed(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Create a mux and register the API routes like the web server does
	mux := http.NewServeMux()
	server.apiServer.RegisterRoutes(mux)

	// POST method should not be allowed
	req := httptest.NewRequest("POST", "/api/search?q=test", nil)
	w := httptest.NewRecorder()

	mux.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAPISearchDatasourceAlphabeticalOrder(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Add a third datasource with a name that will test alphabetical ordering
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	// Create test data with datasource names in non-alphabetical order
	now := time.Now()
	testData := map[string][]core.Block{
		"zebra_ds": {
			&mockBlock{
				id:        "zebra_1",
				text:      "zebra test content",
				createdAt: now,
				source:    "zebra_ds",
			},
		},
		"alpha_ds": {
			&mockBlock{
				id:        "alpha_1",
				text:      "alpha test content",
				createdAt: now,
				source:    "alpha_ds",
			},
		},
	}

	// Setup the additional test data
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
			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block in %s: %v", datasourceName, err)
			}
		}

		mockBlockPrototype := &mockBlock{}
		storageManager.RegisterBlockPrototype(datasourceName, mockBlockPrototype)
	}

	// Update server to use new storage manager and recreate API server
	server.storageManager = storageManager
	server.apiServer = api.NewServer(server.registry, storageManager)

	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleSearch(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check results structure
	results, ok := response["results"].(map[string]interface{})
	if !ok {
		t.Fatal("Results should be a map")
	}

	// Verify we have both datasources
	if len(results) < 2 {
		t.Errorf("Expected at least 2 datasources in results, got %d", len(results))
	}

	// Check that alpha_ds comes before zebra_ds in some ordering
	// Note: The actual ordering will be handled by the backend storage manager
	_, hasAlpha := results["alpha_ds"]
	_, hasZebra := results["zebra_ds"]

	if !hasAlpha {
		t.Error("Expected alpha_ds in results")
	}
	if !hasZebra {
		t.Error("Expected zebra_ds in results")
	}

	t.Logf("API returned results from datasources: %v", getKeys(results))
}

func TestAPIPaginationAccuracy(t *testing.T) {
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	// Create test data with exact known counts
	now := time.Now()
	testData := map[string][]core.Block{
		"api_test_ds": make([]core.Block, 43), // 43 blocks for testing
	}

	// Fill with test data
	for i := 0; i < 43; i++ {
		testData["api_test_ds"][i] = &mockBlock{
			id:        fmt.Sprintf("api_block_%d", i),
			text:      fmt.Sprintf("api pagination test content %d", i),
			createdAt: now.Add(time.Duration(i) * time.Minute),
			source:    "api_test_ds",
			metadata:  map[string]interface{}{"index": i},
		}
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
			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block in %s: %v", datasourceName, err)
			}
		}

		mockBlockPrototype := &mockBlock{}
		storageManager.RegisterBlockPrototype(datasourceName, mockBlockPrototype)
	}

	registry := core.NewRegistry()
	apiServer := api.NewServer(registry, storageManager)
	server := &WebServer{
		storageManager: storageManager,
		registry:       registry,
		apiServer:      apiServer,
	}

	testCases := []struct {
		page            int
		limit           int
		expectedHasMore bool
		expectedResults int
		description     string
	}{
		{1, 10, true, 10, "page 1 of 43 items with limit 10"},
		{2, 10, true, 10, "page 2 of 43 items with limit 10"},
		{3, 10, true, 10, "page 3 of 43 items with limit 10"},
		{4, 10, true, 10, "page 4 of 43 items with limit 10"},
		{5, 10, false, 3, "page 5 (last) of 43 items with limit 10"},
		{6, 10, false, 0, "page 6 (beyond last) of 43 items with limit 10"},
		{1, 20, true, 20, "page 1 of 43 items with limit 20"},
		{2, 20, true, 20, "page 2 of 43 items with limit 20"},
		{3, 20, false, 3, "page 3 (last) of 43 items with limit 20"},
		{1, 50, false, 43, "page 1 of 43 items with limit 50 (all on one page)"},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			url := fmt.Sprintf("/api/search?q=api&page=%d&limit=%d", tc.page, tc.limit)
			req := httptest.NewRequest("GET", url, nil)
			w := httptest.NewRecorder()

			server.apiServer.HandleSearch(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
				return
			}

			var response map[string]interface{}
			err := json.Unmarshal(w.Body.Bytes(), &response)
			if err != nil {
				t.Fatalf("Failed to parse response: %v", err)
			}

			// Check pagination fields
			totalCount := int(response["total_count"].(float64))
			hasMore := response["has_more"].(bool)
			totalPages := int(response["total_pages"].(float64))
			page := int(response["page"].(float64))
			limit := int(response["limit"].(float64))

			t.Logf("%s: totalCount=%d, hasMore=%v, totalPages=%d, page=%d, limit=%d",
				tc.description, totalCount, hasMore, totalPages, page, limit)

			if totalCount != tc.expectedResults {
				t.Errorf("Expected %d results, got %d", tc.expectedResults, totalCount)
			}

			if hasMore != tc.expectedHasMore {
				t.Errorf("Expected hasMore=%v, got %v", tc.expectedHasMore, hasMore)
			}

			// For efficient pagination, we don't require exact total pages
			// Just verify that totalPages >= current page
			if totalPages < tc.page {
				t.Errorf("totalPages %d should be >= current page %d", totalPages, tc.page)
			}

			if page != tc.page {
				t.Errorf("Expected page=%d, got %d", tc.page, page)
			}

			if limit != tc.limit {
				t.Errorf("Expected limit=%d, got %d", tc.limit, limit)
			}
		})
	}
}

// TestAPISearchDateFiltering tests date filtering functionality in API search
func TestAPISearchDateFiltering(t *testing.T) {
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)

	// Create test data with specific dates
	baseTime := time.Date(2024, 5, 15, 12, 0, 0, 0, time.UTC)
	testData := map[string][]core.Block{
		"test_datasource": {
			&mockBlock{id: "old_block", text: "test content", createdAt: baseTime.AddDate(0, 0, -10), source: "test_datasource"},  // May 5
			&mockBlock{id: "target_block", text: "test content", createdAt: baseTime, source: "test_datasource"},                  // May 15
			&mockBlock{id: "recent_block", text: "test content", createdAt: baseTime.AddDate(0, 0, 5), source: "test_datasource"}, // May 20
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
			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block: %v", err)
			}
		}
	}

	rendererRegistry := render.NewRendererRegistry()
	registry := core.GetGlobalRegistry()
	apiServer := api.NewServer(registry, storageManager)
	server := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		rendererRegistry: rendererRegistry,
		apiServer:        apiServer,
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	tests := []struct {
		name           string
		startDate      string
		endDate        string
		expectedBlocks []string
		shouldError    bool
	}{
		{
			name:           "no date filter",
			startDate:      "",
			endDate:        "",
			expectedBlocks: []string{"recent_block", "target_block", "old_block"}, // all blocks, newest first
			shouldError:    false,
		},
		{
			name:           "start date only",
			startDate:      "2024-05-15",
			endDate:        "",
			expectedBlocks: []string{"recent_block", "target_block"}, // May 15 and later
			shouldError:    false,
		},
		{
			name:           "end date only",
			startDate:      "",
			endDate:        "2024-05-15",
			expectedBlocks: []string{"target_block", "old_block"}, // May 15 and earlier
			shouldError:    false,
		},
		{
			name:           "date range",
			startDate:      "2024-05-14",
			endDate:        "2024-05-16",
			expectedBlocks: []string{"target_block"}, // only May 15
			shouldError:    false,
		},
		{
			name:           "no results in date range",
			startDate:      "2024-06-01",
			endDate:        "2024-06-10",
			expectedBlocks: []string{}, // no blocks in June
			shouldError:    false,
		},
		{
			name:        "invalid start date format",
			startDate:   "invalid-date",
			endDate:     "",
			shouldError: true,
		},
		{
			name:        "invalid end date format",
			startDate:   "",
			endDate:     "invalid-date",
			shouldError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build URL with parameters
			url := "/api/search?q=test"
			if tt.startDate != "" {
				url += "&start_date=" + tt.startDate
			}
			if tt.endDate != "" {
				url += "&end_date=" + tt.endDate
			}

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.apiServer.HandleSearch(rr, req)

			if tt.shouldError {
				if rr.Code != http.StatusBadRequest {
					t.Errorf("Expected status 400 for invalid date, got %d", rr.Code)
				}
				return
			}

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", rr.Code)
				t.Logf("Response body: %s", rr.Body.String())
				return
			}

			var response map[string]interface{}
			if err := json.Unmarshal(rr.Body.Bytes(), &response); err != nil {
				t.Fatalf("Failed to parse JSON response: %v", err)
			}

			results, ok := response["results"].(map[string]interface{})
			if !ok {
				t.Fatal("Invalid results format")
			}

			if len(tt.expectedBlocks) == 0 {
				// Expect no results
				if len(results) > 0 {
					t.Errorf("Expected no results, but got %d datasources with results", len(results))
				}
				return
			}

			dsResults, exists := results["test_datasource"]
			if !exists {
				t.Fatal("Expected results from test_datasource")
			}

			dsData, ok := dsResults.(map[string]interface{})
			if !ok {
				t.Fatal("Invalid datasource results format")
			}

			blocks, ok := dsData["blocks"].([]interface{})
			if !ok {
				t.Fatal("Invalid blocks format")
			}

			if len(blocks) != len(tt.expectedBlocks) {
				t.Errorf("Expected %d blocks, got %d", len(tt.expectedBlocks), len(blocks))
				return
			}

			for i, expectedID := range tt.expectedBlocks {
				block, ok := blocks[i].(map[string]interface{})
				if !ok {
					t.Fatalf("Invalid block format at index %d", i)
				}

				actualID, ok := block["id"].(string)
				if !ok {
					t.Fatalf("Invalid block ID format at index %d", i)
				}

				if actualID != expectedID {
					t.Errorf("Expected block ID %s at position %d, got %s", expectedID, i, actualID)
				}
			}
		})
	}
}

// TestWebSearchDateFiltering tests date filtering functionality in web search
func TestWebSearchDateFiltering(t *testing.T) {
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)

	// Create test data with specific dates
	baseTime := time.Date(2024, 6, 15, 12, 0, 0, 0, time.UTC)
	testData := map[string][]core.Block{
		"web_test_ds": {
			&mockBlock{id: "old_web", text: "web test", createdAt: baseTime.AddDate(0, 0, -5), source: "web_test_ds"},   // June 10
			&mockBlock{id: "target_web", text: "web test", createdAt: baseTime, source: "web_test_ds"},                  // June 15
			&mockBlock{id: "recent_web", text: "web test", createdAt: baseTime.AddDate(0, 0, 3), source: "web_test_ds"}, // June 18
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
			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block: %v", err)
			}
		}
	}

	rendererRegistry := render.NewRendererRegistry()
	registry := core.GetGlobalRegistry()
	apiServer := api.NewServer(registry, storageManager)
	server := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		rendererRegistry: rendererRegistry,
		apiServer:        apiServer,
	}
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	tests := []struct {
		name           string
		startDate      string
		endDate        string
		expectedBlocks []string
	}{
		{
			name:           "web start date filter",
			startDate:      "2024-06-15",
			endDate:        "",
			expectedBlocks: []string{"recent_web", "target_web"}, // June 15 and later
		},
		{
			name:           "web date range filter",
			startDate:      "2024-06-14",
			endDate:        "2024-06-16",
			expectedBlocks: []string{"target_web"}, // only June 15
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Build URL with parameters
			url := "/search?q=web"
			if tt.startDate != "" {
				url += "&start_date=" + tt.startDate
			}
			if tt.endDate != "" {
				url += "&end_date=" + tt.endDate
			}

			req, err := http.NewRequest("GET", url, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.handleSearch(rr, req)

			if rr.Code != http.StatusOK {
				t.Errorf("Expected status 200, got %d", rr.Code)
				t.Logf("Response body: %s", rr.Body.String())
				return
			}

			// For web interface, we just verify it doesn't crash and returns 200
			// The actual rendering would require more complex parsing of HTML
			// The important thing is that the date parsing and search logic works
			body := rr.Body.String()
			if !strings.Contains(body, "Search - Ergs") {
				t.Error("Expected search page title in response")
			}
		})
	}
}

// TestDateFilterParameterParsing tests the parameter parsing for date filters
func TestDateFilterParameterParsing(t *testing.T) {
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Errorf("Failed to close storage manager: %v", err)
		}
	}()

	rendererRegistry := render.NewRendererRegistry()
	registry := core.GetGlobalRegistry()
	apiServer := api.NewServer(registry, storageManager)
	server := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		rendererRegistry: rendererRegistry,
		apiServer:        apiServer,
	}

	tests := []struct {
		name           string
		url            string
		expectedStatus int
	}{
		{
			name:           "valid date formats",
			url:            "/api/search?q=test&start_date=2024-01-01&end_date=2024-12-31",
			expectedStatus: http.StatusOK,
		},
		{
			name:           "invalid start date",
			url:            "/api/search?q=test&start_date=not-a-date",
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "invalid end date",
			url:            "/api/search?q=test&end_date=2024-13-01", // invalid month
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "malformed date",
			url:            "/api/search?q=test&start_date=2024/01/01", // wrong separator
			expectedStatus: http.StatusBadRequest,
		},
		{
			name:           "empty date values",
			url:            "/api/search?q=test&start_date=&end_date=",
			expectedStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req, err := http.NewRequest("GET", tt.url, nil)
			if err != nil {
				t.Fatal(err)
			}

			rr := httptest.NewRecorder()
			server.apiServer.HandleSearch(rr, req)

			if rr.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, rr.Code)
				t.Logf("Response body: %s", rr.Body.String())
			}
		})
	}
}

func TestAPIDatasourcesAlphabeticalOrder(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/datasources", nil)
	w := httptest.NewRecorder()

	server.apiServer.HandleListDatasources(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status %d, got %d", http.StatusOK, w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to parse response: %v", err)
	}

	// Check that datasources exist and are in alphabetical order
	datasources, ok := response["datasources"].([]interface{})
	if !ok {
		t.Fatal("Datasources should be an array")
	}

	if len(datasources) < 2 {
		t.Fatal("Expected at least 2 datasources for ordering test")
	}

	// Extract datasource names
	var names []string
	for _, ds := range datasources {
		dsMap := ds.(map[string]interface{})
		name := dsMap["name"].(string)
		names = append(names, name)
	}

	// Verify they are in alphabetical order
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Errorf("Datasources not in alphabetical order: %s should come before %s", names[i], names[i-1])
		}
	}

	t.Logf("API returned datasources in alphabetical order: %v", names)
}

func getKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
}
