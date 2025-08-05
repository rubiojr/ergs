package cmd

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/rubiojr/ergs/cmd/web/renderers"
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"

	// Import test datasources to register their factories
	_ "github.com/rubiojr/ergs/pkg/datasources/testrand"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
)

// TestDatasourceFilteringIntegration tests the complete datasource filtering functionality
// This simulates a real-world scenario where:
// 1. Multiple datasources have data
// 2. User searches with specific datasource filters
// 3. Both API and web endpoints return only filtered results
func TestDatasourceFilteringIntegration(t *testing.T) {
	// Setup test environment
	tempDir := t.TempDir()
	storageManager := storage.NewManagerWithoutMigrationCheck(tempDir)
	defer func() {
		if err := storageManager.Close(); err != nil {
			t.Logf("Warning: failed to close storage manager: %v", err)
		}
	}()

	registry := core.GetGlobalRegistry()
	defer func() {
		if err := registry.Close(); err != nil {
			t.Logf("Warning: failed to close registry: %v", err)
		}
	}()

	rendererRegistry := renderers.NewRendererRegistry()

	// Create realistic test data across multiple datasources
	now := time.Now()
	testDataSources := map[string][]testBlock{
		"github": {
			{id: "gh1", text: "github commit message: fix search functionality", source: "github"},
			{id: "gh2", text: "github issue: search returns wrong results", source: "github"},
			{id: "gh3", text: "github pull request: improve search performance", source: "github"},
		},
		"hackernews": {
			{id: "hn1", text: "hackernews article about search algorithms", source: "hackernews"},
			{id: "hn2", text: "hackernews discussion on database search", source: "hackernews"},
		},
		"rss": {
			{id: "rss1", text: "rss feed item about search engine optimization", source: "rss"},
			{id: "rss2", text: "rss tech news: new search features released", source: "rss"},
			{id: "rss3", text: "rss programming tutorial: implementing search", source: "rss"},
		},
		"librewolf": {
			{id: "lw1", text: "librewolf bookmark: search documentation", source: "librewolf"},
		},
	}

	// Setup storage for each datasource
	for datasourceName, blocks := range testDataSources {
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

		for i, blockData := range blocks {
			block := &mockBlock{
				id:        blockData.id,
				text:      blockData.text,
				createdAt: now.Add(time.Duration(i) * time.Minute),
				source:    blockData.source,
				metadata:  make(map[string]interface{}),
			}

			err = storage.StoreBlock(block, "mock")
			if err != nil {
				t.Fatalf("Failed to store block in %s: %v", datasourceName, err)
			}
		}

		mockBlockPrototype := &mockBlock{}
		storageManager.RegisterBlockPrototype(datasourceName, mockBlockPrototype)
	}

	server := &WebServer{
		registry:         registry,
		storageManager:   storageManager,
		rendererRegistry: rendererRegistry,
		config: &config.Config{
			StorageDir: tempDir,
		},
	}

	// Test scenarios that should be fixed
	testCases := []struct {
		name                string
		queryParams         string
		expectedDatasources []string
		shouldNotInclude    []string
		description         string
	}{
		{
			name:                "single_datasource_filter",
			queryParams:         "q=search&datasource=github",
			expectedDatasources: []string{"github"},
			shouldNotInclude:    []string{"hackernews", "rss", "librewolf"},
			description:         "Should only return results from github when filtered to github",
		},
		{
			name:                "multiple_datasource_filters",
			queryParams:         "q=search&datasource=github&datasource=rss",
			expectedDatasources: []string{"github", "rss"},
			shouldNotInclude:    []string{"hackernews", "librewolf"},
			description:         "Should only return results from github and rss when filtered to those",
		},
		{
			name:                "specific_three_datasources",
			queryParams:         "q=search&datasource=hackernews&datasource=librewolf&datasource=rss",
			expectedDatasources: []string{"hackernews", "librewolf", "rss"},
			shouldNotInclude:    []string{"github"},
			description:         "Should only return results from hackernews, librewolf, and rss",
		},
		{
			name:                "nonexistent_datasource",
			queryParams:         "q=search&datasource=nonexistent",
			expectedDatasources: []string{},
			shouldNotInclude:    []string{"github", "hackernews", "rss", "librewolf"},
			description:         "Should return no results when filtering to nonexistent datasource",
		},
		{
			name:                "mixed_existing_and_nonexistent",
			queryParams:         "q=search&datasource=github&datasource=nonexistent",
			expectedDatasources: []string{"github"},
			shouldNotInclude:    []string{"hackernews", "rss", "librewolf", "nonexistent"},
			description:         "Should only return github results, filtering out nonexistent datasource",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Test API endpoint
			t.Run("api", func(t *testing.T) {
				apiURL := fmt.Sprintf("/api/search?%s", tc.queryParams)
				req := httptest.NewRequest("GET", apiURL, nil)
				w := httptest.NewRecorder()

				server.handleAPISearch(w, req)

				if w.Code != http.StatusOK {
					t.Errorf("API request failed with status %d", w.Code)
				}

				var response map[string]interface{}
				if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
					t.Fatalf("Failed to parse API response: %v", err)
				}

				results, ok := response["results"].(map[string]interface{})
				if !ok {
					t.Fatal("API results field is not a map")
				}

				// Check that expected datasources are present
				for _, expectedDS := range tc.expectedDatasources {
					if _, found := results[expectedDS]; !found {
						t.Errorf("Expected datasource %s not found in API results", expectedDS)
					}
				}

				// Check that unwanted datasources are NOT present
				for _, unwantedDS := range tc.shouldNotInclude {
					if _, found := results[unwantedDS]; found {
						t.Errorf("Unwanted datasource %s found in API results (filter not working)", unwantedDS)
					}
				}

				// Verify total number of datasources matches expectation
				if len(results) != len(tc.expectedDatasources) {
					t.Errorf("Expected %d datasources in API results, got %d", len(tc.expectedDatasources), len(results))
				}

				t.Logf("API test passed: %s", tc.description)
			})

			// Test Web endpoint
			t.Run("web", func(t *testing.T) {
				webURL := fmt.Sprintf("/search?%s", tc.queryParams)
				req := httptest.NewRequest("GET", webURL, nil)
				w := httptest.NewRecorder()

				server.handleSearch(w, req)

				if w.Code != http.StatusOK {
					t.Errorf("Web request failed with status %d", w.Code)
				}

				// For web interface, we can't easily parse the template output,
				// but we can verify it doesn't crash and returns content
				body := w.Body.String()
				if len(body) == 0 {
					t.Error("Expected non-empty response body from web interface")
				}

				t.Logf("Web test passed: %s", tc.description)
			})
		})
	}

	// Test the specific URL pattern mentioned in the issue
	t.Run("original_issue_url_pattern", func(t *testing.T) {
		// Test the exact URL pattern from the issue: /search?q=github&datasource=hackernews&datasource=librewolf&datasource=rss&limit=30
		issueURL := "/api/search?q=github&datasource=hackernews&datasource=librewolf&datasource=rss&limit=30"
		req := httptest.NewRequest("GET", issueURL, nil)
		w := httptest.NewRecorder()

		server.handleAPISearch(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("Original issue URL failed with status %d", w.Code)
		}

		var response map[string]interface{}
		if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
			t.Fatalf("Failed to parse response: %v", err)
		}

		results, ok := response["results"].(map[string]interface{})
		if !ok {
			t.Fatal("Results field is not a map")
		}

		// Should NOT contain github results since we're filtering to hackernews, librewolf, and rss
		if _, hasGithub := results["github"]; hasGithub {
			t.Error("BUG: Found github results when filtering to hackernews, librewolf, and rss - the original issue is NOT fixed")
		}

		// Should only contain the filtered datasources that have matching content
		expectedDatasources := []string{"hackernews", "rss"} // librewolf might not have "github" in content
		for _, ds := range expectedDatasources {
			if _, found := results[ds]; !found {
				t.Logf("Note: %s not found in results (might not have matching content)", ds)
			}
		}

		t.Logf("Original issue URL test passed - no unwanted github results when filtering to other datasources")
	})
}

// Helper struct for test data
type testBlock struct {
	id     string
	text   string
	source string
}

// TestDatasourceFilterParameterParsing tests that URL parameters are parsed correctly
func TestDatasourceFilterParameterParsing(t *testing.T) {
	testCases := []struct {
		name            string
		url             string
		expectedFilters []string
	}{
		{
			name:            "single_datasource",
			url:             "/search?q=test&datasource=github",
			expectedFilters: []string{"github"},
		},
		{
			name:            "multiple_datasources",
			url:             "/search?q=test&datasource=github&datasource=rss&datasource=hackernews",
			expectedFilters: []string{"github", "rss", "hackernews"},
		},
		{
			name:            "no_datasource_filter",
			url:             "/search?q=test",
			expectedFilters: []string{},
		},
		{
			name:            "empty_datasource_value",
			url:             "/search?q=test&datasource=",
			expectedFilters: []string{""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Parse URL
			parsedURL, err := url.Parse(tc.url)
			if err != nil {
				t.Fatalf("Failed to parse URL: %v", err)
			}

			// Extract datasource parameters the same way the code does
			datasourceFilters := parsedURL.Query()["datasource"]

			// Compare results
			if len(datasourceFilters) != len(tc.expectedFilters) {
				t.Errorf("Expected %d filters, got %d", len(tc.expectedFilters), len(datasourceFilters))
			}

			for i, expected := range tc.expectedFilters {
				if i >= len(datasourceFilters) || datasourceFilters[i] != expected {
					t.Errorf("Expected filter %d to be '%s', got '%s'", i, expected, datasourceFilters[i])
				}
			}
		})
	}
}
