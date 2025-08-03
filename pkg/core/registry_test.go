package core

import (
	"context"
	"testing"
	"time"
)

func TestRegistryBasicFunctionality(t *testing.T) {
	// Test that we can create independent registry instances
	registry1 := NewRegistry()
	registry2 := NewRegistry()

	// Register a test prototype in registry1
	testPrototype := &mockTestDatasource{}

	err := registry1.RegisterPrototype("test-factory", testPrototype)
	if err != nil {
		t.Fatalf("Failed to register factory: %v", err)
	}

	// Create a datasource in registry1
	err = registry1.CreateDatasource("test-isolation", "test-factory", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource in registry1: %v", err)
	}

	// Verify it doesn't exist in registry2 (they should be independent instances)
	datasources2 := registry2.GetAllDatasources()
	if _, exists := datasources2["test-isolation"]; exists {
		t.Error("Datasource should not exist in registry2 - registries should be independent")
	}
}

func TestFactoryRegistration(t *testing.T) {
	// Test that we can register a new prototype
	testPrototype := &mockTestDatasource{}

	// Register the prototype
	RegisterDatasourcePrototype("test-factory", testPrototype)

	// Get a new registry and verify the prototype is available
	registry := GetGlobalRegistry()
	err := registry.CreateDatasource("test-instance", "test-factory", nil)
	if err != nil {
		t.Errorf("Failed to create datasource with registered prototype: %v", err)
	}

	// Verify the datasource was created
	datasources := registry.GetAllDatasources()
	if _, exists := datasources["test-instance"]; !exists {
		t.Error("Test datasource should exist after creation")
	}
}

// Mock datasource for testing
type mockTestDatasource struct {
	instanceName string
}

func (m *mockTestDatasource) Type() string { return "test-factory" }
func (m *mockTestDatasource) Name() string {
	if m.instanceName != "" {
		return m.instanceName
	}
	return "test-datasource"
}
func (m *mockTestDatasource) FetchBlocks(ctx context.Context, blockCh chan<- Block) error {
	return nil
}
func (m *mockTestDatasource) Schema() map[string]any {
	return map[string]any{"test": "TEXT"}
}
func (m *mockTestDatasource) BlockPrototype() Block {
	return &mockTestBlock{}
}
func (m *mockTestDatasource) ConfigType() interface{} {
	return &mockTestConfig{}
}
func (m *mockTestDatasource) SetConfig(config interface{}) error {
	return nil
}
func (m *mockTestDatasource) GetConfig() interface{} {
	return &mockTestConfig{}
}
func (m *mockTestDatasource) Close() error {
	return nil
}
func (m *mockTestDatasource) Factory(instanceName string, config interface{}) (Datasource, error) {
	return &mockTestDatasource{instanceName: instanceName}, nil
}

type mockTestBlock struct{}

func (b *mockTestBlock) ID() string                       { return "test-id" }
func (b *mockTestBlock) Text() string                     { return "test text" }
func (b *mockTestBlock) CreatedAt() time.Time             { return time.Now() }
func (b *mockTestBlock) Source() string                   { return "test" }
func (b *mockTestBlock) Type() string                     { return "test" }
func (b *mockTestBlock) Metadata() map[string]interface{} { return make(map[string]interface{}) }
func (b *mockTestBlock) PrettyText() string               { return "test pretty text" }
func (b *mockTestBlock) Summary() string                  { return "test summary" }
func (b *mockTestBlock) Factory(genericBlock *GenericBlock, source string) Block {
	return &mockTestBlock{}
}

type mockTestConfig struct{}
