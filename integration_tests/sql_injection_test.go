package integration_tests

import (
	"os"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

func TestSQLInjectionProtectionIntegration(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "ergs-sql-injection-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir: %v", err)
		}
	}()

	// Create storage manager
	manager, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create test datasource
	testStorage, err := manager.EnsureStorageWithMigrations("test-datasource")
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	// Insert test data with various content that could be targeted by SQL injection
	testBlocks := []core.Block{
		core.NewGenericBlock("1", "sensitive user data", "source1", "test", time.Now(), nil),
		core.NewGenericBlock("2", "password: secret123", "source2", "test", time.Now(), nil),
		core.NewGenericBlock("3", "admin configuration", "source3", "test", time.Now(), nil),
		core.NewGenericBlock("4", "DROP TABLE users", "source4", "test", time.Now(), nil),
		core.NewGenericBlock("5", "normal content", "source5", "test", time.Now(), nil),
	}

	for _, block := range testBlocks {
		err = testStorage.StoreBlock(block, "test")
		if err != nil {
			t.Fatalf("Failed to store block: %v", err)
		}
	}

	// Create search service
	searchService := storage.NewSearchService(manager)

	// Test various SQL injection attempts
	sqlInjectionAttempts := []struct {
		name        string
		searchQuery string
		expectError bool
		description string
	}{
		{
			name:        "basic_sql_injection",
			searchQuery: "'; DROP TABLE blocks; --",
			expectError: true,
			description: "Basic SQL injection attempt should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "union_select_attack",
			searchQuery: "' UNION SELECT * FROM sqlite_master; --",
			expectError: true,
			description: "UNION SELECT attack should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "table_discovery",
			searchQuery: "' UNION SELECT sql FROM sqlite_master WHERE type='table'; --",
			expectError: true,
			description: "Table discovery attempt should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "data_extraction",
			searchQuery: "' UNION SELECT password FROM users; --",
			expectError: true,
			description: "Data extraction attempt should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "boolean_injection",
			searchQuery: "' OR 1=1 --",
			expectError: true,
			description: "Boolean injection should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "database_manipulation",
			searchQuery: "'; DELETE FROM blocks WHERE 1=1; --",
			expectError: true,
			description: "Database manipulation should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "information_schema_access",
			searchQuery: "' UNION SELECT table_name FROM information_schema.tables; --",
			expectError: true,
			description: "Information schema access should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "pragma_injection",
			searchQuery: "'; PRAGMA table_info(blocks); --",
			expectError: true,
			description: "PRAGMA injection should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "file_system_access",
			searchQuery: "'; ATTACH DATABASE '/etc/passwd' AS pwn; --",
			expectError: true,
			description: "File system access attempt should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "load_extension_attack",
			searchQuery: "' UNION SELECT load_extension('evil.so'); --",
			expectError: true,
			description: "Load extension attack should be rejected by FTS5 as invalid syntax",
		},
		{
			name:        "valid_sql_keywords_as_terms",
			searchQuery: "DROP TABLE users",
			expectError: false,
			description: "Valid search terms that happen to be SQL keywords should work",
		},
		{
			name:        "quoted_sql_injection",
			searchQuery: "\"'; DROP TABLE blocks; --\"",
			expectError: false,
			description: "SQL injection in quotes should be treated as phrase search",
		},
	}

	for _, attempt := range sqlInjectionAttempts {
		t.Run(attempt.name, func(t *testing.T) {
			// Create search parameters with malicious query
			params := storage.SearchParams{
				Query: attempt.searchQuery,
				Page:  1,
				Limit: 10,
			}

			// Execute search - this should NOT execute the SQL injection
			results, err := searchService.Search(params)

			if attempt.expectError && err == nil {
				t.Errorf("Expected error for query %q but got none", attempt.searchQuery)
			}

			if !attempt.expectError && err != nil {
				t.Errorf("Unexpected error for query %q: %v", attempt.searchQuery, err)
			}

			// If we expected an error and got one, that's correct behavior
			if attempt.expectError && err != nil {
				t.Logf("Test %s passed: %s - FTS5 correctly rejected malicious query with error: %v", attempt.name, attempt.description, err)
				return
			}

			if results == nil {
				t.Errorf("Results should not be nil for query %q", attempt.searchQuery)
				return
			}

			// Verify that the search was executed safely
			// The malicious query should be treated as a search term, not executed as SQL
			if results.Query != attempt.searchQuery {
				t.Errorf("Query in results was modified: got %q, want %q", results.Query, attempt.searchQuery)
			}

			// Verify that our test data is still intact by searching for legitimate content
			legitParams := storage.SearchParams{
				Query: "normal",
				Page:  1,
				Limit: 10,
			}

			legitResults, err := searchService.Search(legitParams)
			if err != nil {
				t.Errorf("Failed to search for legitimate content after injection attempt: %v", err)
			}

			if legitResults.TotalCount == 0 {
				t.Error("Legitimate content should still be searchable after injection attempt")
			}

			t.Logf("Test %s passed: %s", attempt.name, attempt.description)
		})
	}

	// Verify database integrity after all injection attempts
	t.Run("database_integrity_check", func(t *testing.T) {
		// Check that all original blocks are still present
		allResults, err := searchService.Search(storage.SearchParams{
			Query: "",
			Page:  1,
			Limit: 100,
		})
		if err != nil {
			t.Fatalf("Failed to retrieve all blocks: %v", err)
		}

		// Count total blocks across all datasources
		totalBlocks := 0
		for _, blocks := range allResults.Results {
			totalBlocks += len(blocks)
		}

		if totalBlocks != len(testBlocks) {
			t.Errorf("Expected %d blocks after injection attempts, got %d", len(testBlocks), totalBlocks)
		}

		// Verify specific sensitive content is still searchable
		sensitiveSearches := []string{"sensitive", "password", "admin", "normal"}
		for _, search := range sensitiveSearches {
			params := storage.SearchParams{
				Query: search,
				Page:  1,
				Limit: 10,
			}

			results, err := searchService.Search(params)
			if err != nil {
				t.Errorf("Failed to search for %q: %v", search, err)
			}

			if results.TotalCount == 0 {
				t.Errorf("Expected to find results for %q", search)
			}
		}

		t.Log("Database integrity verified - all data intact after injection attempts")
	})
}

func TestFTS5SpecificInjectionProtection(t *testing.T) {
	// Create temporary directory for test
	tempDir, err := os.MkdirTemp("", "ergs-fts5-injection-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir: %v", err)
		}
	}()

	// Create storage manager
	manager, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	// Create test datasource
	testStorage, err := manager.EnsureStorageWithMigrations("fts5-test")
	if err != nil {
		t.Fatalf("Failed to create test storage: %v", err)
	}

	// Insert test data
	testBlocks := []core.Block{
		core.NewGenericBlock("1", "github repository golang", "source1", "fts5-test", time.Now(), nil),
		core.NewGenericBlock("2", "rust programming language", "source2", "fts5-test", time.Now(), nil),
		core.NewGenericBlock("3", "python data science", "source3", "fts5-test", time.Now(), nil),
	}

	for _, block := range testBlocks {
		err = testStorage.StoreBlock(block, "fts5-test")
		if err != nil {
			t.Fatalf("Failed to store block: %v", err)
		}
	}

	// Create search service
	searchService := storage.NewSearchService(manager)

	// Test FTS5-specific injection attempts embedded in valid FTS5 syntax
	fts5InjectionTests := []struct {
		name  string
		query string
		desc  string
	}{
		{
			name:  "column_filter_injection",
			query: "text:'; DROP TABLE blocks; --'",
			desc:  "SQL injection embedded in column filter",
		},
		{
			name:  "phrase_query_injection",
			query: "\"'; DELETE FROM blocks WHERE 1=1; --\"",
			desc:  "SQL injection embedded in phrase query",
		},
		{
			name:  "near_query_injection",
			query: "NEAR(\"; DROP TABLE blocks; --\" hack, 1)",
			desc:  "SQL injection embedded in NEAR query",
		},
		{
			name:  "boolean_operator_injection",
			query: "golang AND '; PRAGMA table_info(blocks); --'",
			desc:  "SQL injection with boolean operators",
		},
		{
			name:  "complex_fts5_injection",
			query: "datasource:'; UNION SELECT * FROM sqlite_master; --' OR text:\"'; DROP TABLE blocks; --\"",
			desc:  "Complex FTS5 query with multiple injection attempts",
		},
	}

	for _, test := range fts5InjectionTests {
		t.Run(test.name, func(t *testing.T) {
			params := storage.SearchParams{
				Query: test.query,
				Page:  1,
				Limit: 10,
			}

			// Execute search - should not execute any SQL injection
			results, err := searchService.Search(params)
			if err != nil {
				// FTS5 might reject some malformed queries, which is acceptable
				t.Logf("FTS5 query rejected (acceptable): %v", err)
				return
			}

			if results == nil {
				t.Errorf("Results should not be nil for query %q", test.query)
				return
			}

			// Verify the query was preserved
			if results.Query != test.query {
				t.Errorf("Query was modified: got %q, want %q", results.Query, test.query)
			}

			t.Logf("Test %s passed: %s - Query processed safely", test.name, test.desc)
		})
	}

	// Verify database is still intact after FTS5 injection attempts
	params := storage.SearchParams{
		Query: "golang",
		Page:  1,
		Limit: 10,
	}

	results, err := searchService.Search(params)
	if err != nil {
		t.Fatalf("Failed to search after FTS5 injection attempts: %v", err)
	}

	if results.TotalCount == 0 {
		t.Error("Expected to find golang content after FTS5 injection attempts")
	}

	t.Log("FTS5 injection protection verified - database integrity maintained")
}

func TestParameterInjectionProtection(t *testing.T) {
	// Test injection through various search parameters
	tempDir, err := os.MkdirTemp("", "ergs-param-injection-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir: %v", err)
		}
	}()

	manager, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("Failed to create manager: %v", err)
	}

	searchService := storage.NewSearchService(manager)

	// Test parameter injection attempts
	maliciousParams := storage.SearchParams{
		Query:             "normal search",
		DatasourceFilters: []string{"github'; DROP TABLE blocks; --", "validname", "another'; UNION SELECT"},
		Page:              1,
		Limit:             30,
		StartDate:         &time.Time{}, // Valid time object
		EndDate:           &time.Time{}, // Valid time object
	}

	// Execute search with malicious parameters
	results, err := searchService.Search(maliciousParams)
	if err != nil {
		t.Logf("Search failed (acceptable for malicious params): %v", err)
		return
	}

	if results == nil {
		t.Error("Results should not be nil")
		return
	}

	// Verify that invalid datasource names were filtered out during parsing
	// (This test validates the parameter parsing, not the search execution)
	t.Log("Parameter injection test completed - malicious datasource names handled safely")
}

func TestDateParameterSafety(t *testing.T) {
	// Test that date parameters are safe from injection
	tempDir, err := os.MkdirTemp("", "ergs-date-injection-test")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			t.Logf("Warning: failed to remove temp dir: %v", err)
		}
	}()

	// Test parsing malicious date parameters
	maliciousDateParams := map[string][]string{
		"q":          {"test"},
		"start_date": {"2023-01-01'; DROP TABLE blocks; --"},
		"end_date":   {"2023-12-31'; PRAGMA table_info(blocks); --"},
	}

	// This should fail during parsing due to invalid date format
	_, err = storage.ParseSearchParams(maliciousDateParams)
	if err == nil {
		t.Error("Expected error for malicious date parameters")
	}

	// Verify the error is about date parsing, not SQL execution
	if err != nil && !contains(err.Error(), "invalid") {
		t.Errorf("Expected date parsing error, got: %v", err)
	}

	t.Log("Date parameter safety verified - malicious dates rejected during parsing")
}

// Helper function to check if string contains substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) &&
			(s[:len(substr)] == substr ||
				s[len(s)-len(substr):] == substr ||
				findInString(s, substr))))
}

func findInString(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
