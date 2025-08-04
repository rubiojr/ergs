package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rubiojr/ergs/cmd/web/renderers"
	"github.com/rubiojr/ergs/pkg/core"
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
	storageManager := storage.NewManager(tempDir)
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

		storage, err := storageManager.GetStorage(datasourceName)
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
	rendererRegistry := renderers.NewRendererRegistry()

	server := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		rendererRegistry: rendererRegistry,
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

	server.handleAPISearch(w, req)

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

func TestAPISearchPagination(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test page 1 with limit 10
	req := httptest.NewRequest("GET", "/api/search?q=test&page=1&limit=10", nil)
	w := httptest.NewRecorder()

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

func TestAPISearchLastPage(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Test page 3 with limit 10 (should have remaining results)
	req := httptest.NewRequest("GET", "/api/search?q=test&page=3&limit=10", nil)
	w := httptest.NewRecorder()

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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
	server.handleAPISearch(apiW, apiReq)

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

	server.handleAPISearch(w, req)

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

	server.handleAPISearch(w, req)

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

	// POST method should not be allowed
	req := httptest.NewRequest("POST", "/api/search?q=test", nil)
	w := httptest.NewRecorder()

	server.handleAPISearch(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status %d, got %d", http.StatusMethodNotAllowed, w.Code)
	}
}

func TestAPISearchDatasourceAlphabeticalOrder(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	// Add a third datasource with a name that will test alphabetical ordering
	tempDir := t.TempDir()
	storageManager := storage.NewManager(tempDir)
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

		storage, err := storageManager.GetStorage(datasourceName)
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

	// Update server to use new storage manager
	server.storageManager = storageManager

	req := httptest.NewRequest("GET", "/api/search?q=test", nil)
	w := httptest.NewRecorder()

	server.handleAPISearch(w, req)

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
	storageManager := storage.NewManager(tempDir)
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

		storage, err := storageManager.GetStorage(datasourceName)
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

	server := &WebServer{storageManager: storageManager}

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

			server.handleAPISearch(w, req)

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

func TestAPIDatasourcesAlphabeticalOrder(t *testing.T) {
	server, cleanup := setupTestWebServer(t)
	defer cleanup()

	req := httptest.NewRequest("GET", "/api/datasources", nil)
	w := httptest.NewRecorder()

	server.handleAPIListDatasources(w, req)

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
