package core

import (
	"fmt"
	"testing"
	"time"
)

type mockBlock struct {
	id        string
	text      string
	createdAt time.Time
	source    string
	metadata  map[string]interface{}
}

func (m *mockBlock) ID() string                       { return m.id }
func (m *mockBlock) Text() string                     { return m.text }
func (m *mockBlock) CreatedAt() time.Time             { return m.createdAt }
func (m *mockBlock) Source() string                   { return m.source }
func (m *mockBlock) Metadata() map[string]interface{} { return m.metadata }
func (m *mockBlock) PrettyText() string {
	// Format metadata using utility function
	metadataInfo := FormatMetadata(m.metadata)

	return fmt.Sprintf("🧪 Mock Block: %s\n  ID: %s\n  Time: %s\n  Source: %s%s",
		m.text, m.id, m.createdAt.Format("2006-01-02 15:04:05"), m.source, metadataInfo)
}

func TestBlockInterface(t *testing.T) {
	now := time.Now()
	block := &mockBlock{
		id:        "test-123",
		text:      "This is a test block",
		createdAt: now,
		source:    "test-source",
		metadata:  map[string]interface{}{"type": "test"},
	}

	// Test basic interface methods
	if block.ID() != "test-123" {
		t.Errorf("Expected ID 'test-123', got '%s'", block.ID())
	}

	if block.Text() != "This is a test block" {
		t.Errorf("Expected text 'This is a test block', got '%s'", block.Text())
	}

	if block.Source() != "test-source" {
		t.Errorf("Expected source 'test-source', got '%s'", block.Source())
	}

	if !block.CreatedAt().Equal(now) {
		t.Errorf("Expected time %v, got %v", now, block.CreatedAt())
	}

	// Test PrettyText formatting
	prettyText := block.PrettyText()
	expectedPrefix := "🧪 Mock Block: This is a test block"
	if len(prettyText) == 0 {
		t.Error("PrettyText should not be empty")
	}

	if prettyText[:len(expectedPrefix)] != expectedPrefix {
		t.Errorf("PrettyText should start with '%s', got '%s'", expectedPrefix, prettyText[:len(expectedPrefix)])
	}

	// Test that PrettyText contains key information
	if !contains(prettyText, "test-123") {
		t.Error("PrettyText should contain the block ID")
	}

	if !contains(prettyText, "test-source") {
		t.Error("PrettyText should contain the source")
	}

	if !contains(prettyText, now.Format("2006-01-02 15:04:05")) {
		t.Error("PrettyText should contain the formatted time")
	}
}

func TestBlockMetadata(t *testing.T) {
	metadata := map[string]interface{}{
		"type":   "test",
		"count":  42,
		"active": true,
		"tags":   []string{"tag1", "tag2"},
	}

	block := &mockBlock{
		id:        "meta-test",
		text:      "Metadata test block",
		createdAt: time.Now(),
		source:    "meta-source",
		metadata:  metadata,
	}

	blockMetadata := block.Metadata()
	if len(blockMetadata) != 4 {
		t.Errorf("Expected 4 metadata items, got %d", len(blockMetadata))
	}

	if blockMetadata["type"] != "test" {
		t.Errorf("Expected metadata type 'test', got '%v'", blockMetadata["type"])
	}

	if blockMetadata["count"] != 42 {
		t.Errorf("Expected metadata count 42, got %v", blockMetadata["count"])
	}

	if blockMetadata["active"] != true {
		t.Errorf("Expected metadata active true, got %v", blockMetadata["active"])
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && s[len(s)-len(substr):] == substr ||
		len(s) > len(substr) && containsSubstring(s, substr)
}

func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
