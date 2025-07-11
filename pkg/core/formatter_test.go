package core

import (
	"strings"
	"testing"
)

func TestFormatMetadata(t *testing.T) {
	tests := []struct {
		name     string
		metadata map[string]interface{}
		expected []string // strings that should be present in output
	}{
		{
			name:     "empty metadata",
			metadata: map[string]interface{}{},
			expected: []string{},
		},
		{
			name: "string metadata",
			metadata: map[string]interface{}{
				"name": "test-repo",
				"desc": "A test repository",
			},
			expected: []string{"name: test-repo", "desc: A test repository"},
		},
		{
			name: "mixed types metadata",
			metadata: map[string]interface{}{
				"count":  42,
				"active": true,
				"score":  3.14,
				"name":   "example",
			},
			expected: []string{"count: 42", "active: true", "score: 3.14", "name: example"},
		},
		{
			name: "long string truncation",
			metadata: map[string]interface{}{
				"long_text": strings.Repeat("a", 150),
			},
			expected: []string{"long_text: " + strings.Repeat("a", 97) + "..."},
		},
		{
			name: "complex type formatting",
			metadata: map[string]interface{}{
				"tags":   []string{"tag1", "tag2"},
				"config": map[string]string{"key": "value"},
			},
			expected: []string{"tags:", "config:"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := FormatMetadata(tt.metadata)

			if len(tt.metadata) == 0 {
				if result != "" {
					t.Errorf("Expected empty result for empty metadata, got: %s", result)
				}
				return
			}

			// Check that result contains metadata header
			if !strings.Contains(result, "Metadata:") {
				t.Errorf("Expected result to contain 'Metadata:', got: %s", result)
			}

			// Check that all expected strings are present
			for _, expected := range tt.expected {
				if !strings.Contains(result, expected) {
					t.Errorf("Expected result to contain '%s', got: %s", expected, result)
				}
			}
		})
	}
}

func TestFormatMetadataConsistency(t *testing.T) {
	metadata := map[string]interface{}{
		"repo_name":       "test/repo",
		"stars":           100,
		"public":          true,
		"description":     "A test repository for validation",
		"very_long_field": strings.Repeat("x", 200),
	}

	result := FormatMetadata(metadata)

	// Verify all keys are present (maps are non-deterministic, so we can't compare exact output)
	expectedKeys := []string{"repo_name", "stars", "public", "description", "very_long_field"}
	for _, key := range expectedKeys {
		if !strings.Contains(result, key+":") {
			t.Errorf("Expected result to contain key '%s', got: %s", key, result)
		}
	}

	// Verify long field is truncated
	if !strings.Contains(result, "...") {
		t.Errorf("Expected long field to be truncated with '...', got: %s", result)
	}

	// Verify metadata header is present
	if !strings.Contains(result, "Metadata:") {
		t.Errorf("Expected result to contain 'Metadata:', got: %s", result)
	}
}
