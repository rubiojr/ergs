package storage

import (
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/db"
)

func TestEscapeFTS5Query(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{
			name:     "simple query without special characters",
			input:    "golang",
			expected: "golang",
		},
		{
			name:     "query with equals sign",
			input:    "config=value",
			expected: "config=value",
		},
		{
			name:     "query with less than",
			input:    "version<2.0",
			expected: "version<2.0",
		},
		{
			name:     "query with greater than",
			input:    "version>1.0",
			expected: "version>1.0",
		},
		{
			name:     "query with exclamation mark",
			input:    "not!found",
			expected: "not!found",
		},
		{
			name:     "query with parentheses",
			input:    "(golang OR rust)",
			expected: "(golang OR rust)",
		},
		{
			name:     "query with double quotes",
			input:    "exact \"phrase\" match",
			expected: "exact \"phrase\" match",
		},
		{
			name:     "query with asterisk",
			input:    "prefix*",
			expected: "prefix*",
		},
		{
			name:     "query with colon",
			input:    "datasource:gasstations",
			expected: "datasource:gasstations",
		},
		{
			name:     "query with caret",
			input:    "boost^2",
			expected: "boost^2",
		},
		{
			name:     "query with minus",
			input:    "include-exclude",
			expected: "include-exclude",
		},
		{
			name:     "query with plus",
			input:    "required+term",
			expected: "required+term",
		},
		{
			name:     "query with multiple special characters",
			input:    "datasource:gasstations AND text:\"fuel price\"",
			expected: "datasource:gasstations AND text:\"fuel price\"",
		},
		{
			name:     "query with nested quotes",
			input:    "search \"nested \"inner\" quotes\"",
			expected: "search \"nested \"inner\" quotes\"",
		},
		{
			name:     "empty query",
			input:    "",
			expected: "",
		},
		{
			name:     "query with only spaces",
			input:    "   ",
			expected: "   ",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := escapeFTS5Query(tt.input)
			if result != tt.expected {
				t.Errorf("escapeFTS5Query(%q) = %q, want %q", tt.input, result, tt.expected)
			}
		})
	}
}

func TestEscapeFTS5QueryConsistency(t *testing.T) {
	// Test that the function is deterministic
	query := "config=value AND (type:repo)"
	result1 := escapeFTS5Query(query)
	result2 := escapeFTS5Query(query)

	if result1 != result2 {
		t.Errorf("escapeFTS5Query should be deterministic, got %q and %q", result1, result2)
	}

	// Test that all queries are preserved as-is
	phraseQuery := "\"already a phrase\""
	result := escapeFTS5Query(phraseQuery)
	expected := "\"already a phrase\""

	if result != expected {
		t.Errorf("escapeFTS5Query(%q) = %q, want %q", phraseQuery, result, expected)
	}
}

func TestFTS5SpecialCharacters(t *testing.T) {
	// Test that special characters are preserved (no escaping)
	specialChars := map[string]string{
		"=": "test=value",
		"<": "test<value",
		">": "test>value",
		"!": "test!value",
		"^": "test^value",
		"-": "test-value",
		"+": "test+value",
	}

	for char, expected := range specialChars {
		input := "test" + char + "value"
		result := escapeFTS5Query(input)
		if result != expected {
			t.Errorf("escapeFTS5Query(%q) = %q, want %q", input, result, expected)
		}
	}
}

func TestFTS5AllowsAllSyntax(t *testing.T) {
	// Test that all FTS5 syntax patterns are preserved as-is
	testCases := []struct {
		name  string
		input string
	}{
		{"column filter datasource", "datasource:gasstations"},
		{"column filter text", "text:golang"},
		{"column filter source", "source:github"},
		{"column filter metadata", "metadata:important"},
		{"phrase query", "\"gas station\""},
		{"boolean AND", "golang AND rust"},
		{"boolean OR", "python OR java"},
		{"boolean NOT", "NOT spam"},
		{"NEAR operator", "golang NEAR rust"},
		{"NEAR with distance", "term1 NEAR/5 term2"},
		{"prefix wildcard", "prefix*"},
		{"simple token", "golang"},
		{"simple multi-word", "simple search"},
		{"complex valid query", "datasource:gasstations AND text:\"fuel price\""},
		{"quoted column", "\"datasource\":github"},
		{"quoted column with operators", "\"datasource\":github + gasstation"},
		{"plus operator", "golang + rust"},
		{"minus operator", "golang - deprecated"},
		{"parentheses grouping", "(golang OR rust) AND active"},
		{"complex quoted column", "\"text\":\"hello world\" AND \"datasource\":github"},
		{"special characters", "term=value<>!^-+"},
		{"complex expression", "config=value AND (type:repo OR type:issue)"},
		{"all operators", "a AND b OR c NOT d + e - f"},
		{"nested quotes", "search \"nested \"inner\" quotes\""},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := escapeFTS5Query(tc.input)
			if result != tc.input {
				t.Errorf("All FTS5 syntax should be preserved: escapeFTS5Query(%q) = %q, want %q", tc.input, result, tc.input)
			}
		})
	}
}

// TestStoreBlocksUpdatedAtPreservedCreatedAt verifies that:
// 1. created_at is preserved across upserts
// 2. updated_at is initialized to created_at then advances on conflict update
func TestStoreBlocksUpdatedAtPreservedCreatedAt(t *testing.T) {
	tempDir := t.TempDir()
	dbPath := tempDir + "/test.db"

	st, err := NewGenericStorage(dbPath, "testds")
	if err != nil {
		t.Fatalf("NewGenericStorage error: %v", err)
	}
	defer func() { _ = st.Close() }()

	if err := db.InitializeDatabase(st.GetDB()); err != nil {
		t.Fatalf("InitializeDatabase error: %v", err)
	}

	createdAt := time.Now().Add(-1 * time.Hour).UTC().Truncate(time.Second)

	block1 := core.NewGenericBlock("block-1", "original text", "srcA", "testds", createdAt, map[string]interface{}{"v": 1})
	if err := st.StoreBlock(block1, "testds"); err != nil {
		t.Fatalf("StoreBlock initial failed: %v", err)
	}

	var created1, updated1 time.Time
	if err := st.GetDB().QueryRow("SELECT created_at, updated_at FROM blocks WHERE id = ?", "block-1").
		Scan(&created1, &updated1); err != nil {
		t.Fatalf("query after first insert failed: %v", err)
	}

	if !created1.Equal(createdAt) {
		t.Errorf("created_at mismatch: got %v want %v", created1, createdAt)
	}
	if !updated1.Equal(created1) {
		t.Errorf("expected updated_at == created_at on first insert, got created_at=%v updated_at=%v", created1, updated1)
	}

	time.Sleep(1500 * time.Millisecond) // ensure second-level resolution difference

	block2 := core.NewGenericBlock("block-1", "modified text", "srcA", "testds", createdAt, map[string]interface{}{"v": 2})
	if err := st.StoreBlock(block2, "testds"); err != nil {
		t.Fatalf("StoreBlock update failed: %v", err)
	}

	var created2, updated2 time.Time
	if err := st.GetDB().QueryRow("SELECT created_at, updated_at FROM blocks WHERE id = ?", "block-1").
		Scan(&created2, &updated2); err != nil {
		t.Fatalf("query after update failed: %v", err)
	}

	if !created2.Equal(created1) {
		t.Errorf("created_at changed unexpectedly: before=%v after=%v", created1, created2)
	}
	if !updated2.After(updated1) {
		t.Errorf("updated_at not advanced: before=%v after=%v", updated1, updated2)
	}
	if time.Until(updated2) > time.Minute {
		t.Errorf("updated_at appears unrealistic (in the future): %v", updated2)
	}
}
