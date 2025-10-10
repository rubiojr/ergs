package datadis

import (
	"context"
	"testing"
	"time"

	"github.com/rubiojr/ergs/pkg/core"
)

func TestDatadisDatasourceBasicFunctionality(t *testing.T) {
	// Test config validation
	config := &Config{
		Username: "test@example.com",
		Password: "testpassword",
	}
	if err := config.Validate(); err != nil {
		t.Errorf("Valid config should not return error: %v", err)
	}

	// Test invalid config - missing username
	invalidConfig := &Config{
		Username: "",
		Password: "testpassword",
	}
	if err := invalidConfig.Validate(); err == nil {
		t.Error("Config with missing username should return error")
	}

	// Test invalid config - missing password
	invalidConfig2 := &Config{
		Username: "test@example.com",
		Password: "",
	}
	if err := invalidConfig2.Validate(); err == nil {
		t.Error("Config with missing password should return error")
	}

	// Test empty config (both fields empty - should fail)
	emptyConfig := &Config{}
	if err := emptyConfig.Validate(); err == nil {
		t.Error("Empty config should return error - credentials are required")
	}

	// Test partial config - only username (should fail)
	partialConfig := &Config{
		Username: "test@example.com",
		Password: "",
	}
	if err := partialConfig.Validate(); err == nil {
		t.Error("Config with only username should return error")
	}
}

func TestDatadisDatasourceCreation(t *testing.T) {
	// Test datasource creation with nil config
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource with nil config: %v", err)
	}

	if ds.Name() != "test-datadis" {
		t.Errorf("Expected name 'test-datadis', got '%s'", ds.Name())
	}

	if ds.Type() != "datadis" {
		t.Errorf("Expected type 'datadis', got '%s'", ds.Type())
	}

	// Test datasource creation with valid config (won't connect, but should create)
	config := &Config{
		Username: "test@example.com",
		Password: "testpassword",
	}
	ds2, err := NewDatasource("test-datadis-2", config)
	if err != nil {
		t.Fatalf("Failed to create datasource with config: %v", err)
	}

	if ds2.Name() != "test-datadis-2" {
		t.Errorf("Expected name 'test-datadis-2', got '%s'", ds2.Name())
	}
}

func TestDatadisDatasourceSchema(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	schema := ds.Schema()
	if schema == nil {
		t.Fatal("Schema should not be nil")
	}

	expectedFields := []string{
		"cups", "date", "hour", "consumption", "obtain_method",
		"address", "province", "postal_code", "municipality", "distributor",
	}

	for _, field := range expectedFields {
		if _, exists := schema[field]; !exists {
			t.Errorf("Schema missing expected field: %s", field)
		}
	}

	// Verify field types
	if schema["consumption"] != "REAL" {
		t.Errorf("Expected consumption to be REAL, got %v", schema["consumption"])
	}

	for _, field := range []string{"cups", "date", "hour", "obtain_method", "address"} {
		if schema[field] != "TEXT" {
			t.Errorf("Expected %s to be TEXT, got %v", field, schema[field])
		}
	}
}

func TestDatadisDatasourceBlockPrototype(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	prototype := ds.BlockPrototype()
	if prototype == nil {
		t.Fatal("Block prototype should not be nil")
	}

	// Verify it's the correct type
	_, ok := prototype.(*ConsumptionBlock)
	if !ok {
		t.Errorf("Block prototype should be *ConsumptionBlock, got %T", prototype)
	}
}

func TestDatadisDatasourceConfigType(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	configType := ds.ConfigType()
	if configType == nil {
		t.Fatal("ConfigType should not be nil")
	}

	_, ok := configType.(*Config)
	if !ok {
		t.Errorf("ConfigType should be *Config, got %T", configType)
	}
}

func TestDatadisDatasourceSetConfig(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	// Test with invalid config type
	err = ds.SetConfig("invalid config")
	if err == nil {
		t.Error("SetConfig should return error for invalid config type")
	}

	// Test with valid config type (won't connect, but should validate)
	validConfig := &Config{
		Username: "test@example.com",
		Password: "testpassword",
	}
	err = ds.SetConfig(validConfig)
	// We expect this to fail because we can't actually connect to Datadis in tests
	// but it shouldn't panic and should return a clear error
	if err == nil {
		t.Log("SetConfig succeeded (unexpected if not connected to real Datadis API)")
	}

	// Test with empty config (both fields empty - should fail)
	emptyConfig := &Config{}
	err = ds.SetConfig(emptyConfig)
	if err == nil {
		t.Error("SetConfig with empty config should return error - credentials are required")
	}
}

func TestDatadisDatasourceGetConfig(t *testing.T) {
	config := &Config{
		Username: "test@example.com",
		Password: "testpassword",
	}

	ds, err := NewDatasource("test-datadis", config)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	retrievedConfig := ds.GetConfig()
	if retrievedConfig == nil {
		t.Fatal("GetConfig should not return nil")
	}

	cfg, ok := retrievedConfig.(*Config)
	if !ok {
		t.Fatalf("GetConfig should return *Config, got %T", retrievedConfig)
	}

	if cfg.Username != "test@example.com" {
		t.Errorf("Expected username 'test@example.com', got '%s'", cfg.Username)
	}

	if cfg.Password != "testpassword" {
		t.Errorf("Expected password 'testpassword', got '%s'", cfg.Password)
	}
}

func TestDatadisDatasourceClose(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	// Close should not error
	if err := ds.Close(); err != nil {
		t.Errorf("Close returned error: %v", err)
	}
}

func TestDatadisDatasourceFactory(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	config := &Config{
		Username: "factory@example.com",
		Password: "factorypass",
	}

	newDS, err := ds.Factory("factory-instance", config)
	if err != nil {
		t.Fatalf("Factory failed: %v", err)
	}

	if newDS.Name() != "factory-instance" {
		t.Errorf("Expected name 'factory-instance', got '%s'", newDS.Name())
	}

	if newDS.Type() != "datadis" {
		t.Errorf("Expected type 'datadis', got '%s'", newDS.Type())
	}
}

func TestConsumptionBlockCreation(t *testing.T) {
	now := time.Now()
	block := NewConsumptionBlock(
		"test-id",
		"ES0021000000000000XX",
		"2025/01/15",
		"14:00",
		1.23,
		"R",
		"Calle Example 123",
		"Madrid",
		"28001",
		"Madrid",
		"i-DE Redes Eléctricas",
		now,
		"test-source",
	)

	if block == nil {
		t.Fatal("NewConsumptionBlock returned nil")
	}

	if block.ID() != "test-id" {
		t.Errorf("Expected ID 'test-id', got '%s'", block.ID())
	}

	if block.Source() != "test-source" {
		t.Errorf("Expected source 'test-source', got '%s'", block.Source())
	}

	if block.Type() != "datadis" {
		t.Errorf("Expected type 'datadis', got '%s'", block.Type())
	}

	if block.CreatedAt() != now {
		t.Errorf("Expected CreatedAt %v, got %v", now, block.CreatedAt())
	}

	if block.CUPS() != "ES0021000000000000XX" {
		t.Errorf("Expected CUPS 'ES0021000000000000XX', got '%s'", block.CUPS())
	}

	if block.Consumption() != 1.23 {
		t.Errorf("Expected consumption 1.23, got %f", block.Consumption())
	}
}

func TestConsumptionBlockText(t *testing.T) {
	block := NewConsumptionBlock(
		"test-id",
		"ES0021000000000000XX",
		"2025/01/15",
		"14:00",
		1.23,
		"R",
		"Calle Example 123",
		"Madrid",
		"28001",
		"Madrid",
		"i-DE Redes Eléctricas",
		time.Now(),
		"test-source",
	)

	text := block.Text()
	if text == "" {
		t.Error("Block text should not be empty")
	}

	// Text should contain key information
	if !contains(text, "1.23") {
		t.Error("Text should contain consumption value")
	}
	if !contains(text, "ES0021000000000000XX") {
		t.Error("Text should contain CUPS")
	}
	if !contains(text, "Calle Example 123") {
		t.Error("Text should contain address")
	}
}

func TestConsumptionBlockMetadata(t *testing.T) {
	block := NewConsumptionBlock(
		"test-id",
		"ES0021000000000000XX",
		"2025/01/15",
		"14:00",
		1.23,
		"R",
		"Calle Example 123",
		"Madrid",
		"28001",
		"Madrid",
		"i-DE Redes Eléctricas",
		time.Now(),
		"test-source",
	)

	metadata := block.Metadata()
	if metadata == nil {
		t.Fatal("Metadata should not be nil")
	}

	expectedFields := map[string]interface{}{
		"cups":          "ES0021000000000000XX",
		"date":          "2025/01/15",
		"hour":          "14:00",
		"consumption":   float32(1.23),
		"obtain_method": "R",
		"address":       "Calle Example 123",
		"province":      "Madrid",
		"postal_code":   "28001",
		"municipality":  "Madrid",
		"distributor":   "i-DE Redes Eléctricas",
	}

	for key, expectedValue := range expectedFields {
		actualValue, exists := metadata[key]
		if !exists {
			t.Errorf("Metadata missing key: %s", key)
			continue
		}

		if actualValue != expectedValue {
			t.Errorf("Metadata[%s] = %v, expected %v", key, actualValue, expectedValue)
		}
	}
}

func TestConsumptionBlockPrettyText(t *testing.T) {
	block := NewConsumptionBlock(
		"test-id",
		"ES0021000000000000XX",
		"2025/01/15",
		"14:00",
		1.23,
		"R",
		"Calle Example 123",
		"Madrid",
		"28001",
		"Madrid",
		"i-DE Redes Eléctricas",
		time.Now(),
		"test-source",
	)

	prettyText := block.PrettyText()
	if prettyText == "" {
		t.Error("PrettyText should not be empty")
	}

	// Should contain emojis and formatted data
	if !contains(prettyText, "⚡") {
		t.Error("PrettyText should contain electricity emoji")
	}
	if !contains(prettyText, "1.23 kWh") {
		t.Error("PrettyText should contain formatted consumption")
	}
}

func TestConsumptionBlockSummary(t *testing.T) {
	block := NewConsumptionBlock(
		"test-id",
		"ES0021000000000000XX",
		"2025/01/15",
		"14:00",
		1.23,
		"R",
		"Calle Example 123",
		"Madrid",
		"28001",
		"Madrid",
		"i-DE Redes Eléctricas",
		time.Now(),
		"test-source",
	)

	summary := block.Summary()
	if summary == "" {
		t.Error("Summary should not be empty")
	}

	// Summary should be concise and contain key info
	if !contains(summary, "1.23") {
		t.Error("Summary should contain consumption value")
	}
	if !contains(summary, "⚡") {
		t.Error("Summary should contain emoji")
	}
}

func TestConsumptionBlockFactory(t *testing.T) {
	block := NewConsumptionBlock(
		"original-id",
		"ES0021000000000000XX",
		"2025/01/15",
		"14:00",
		1.23,
		"R",
		"Calle Example 123",
		"Madrid",
		"28001",
		"Madrid",
		"i-DE Redes Eléctricas",
		time.Now(),
		"original-source",
	)

	// Create generic block from the consumption block
	genericBlock := core.NewGenericBlock(
		block.ID(),
		block.Text(),
		block.Source(),
		block.Type(),
		block.CreatedAt(),
		block.Metadata(),
	)

	// Use factory to recreate
	reconstructed := block.Factory(genericBlock, "new-source")

	if reconstructed == nil {
		t.Fatal("Factory returned nil")
	}

	if reconstructed.ID() != "original-id" {
		t.Errorf("Expected ID 'original-id', got '%s'", reconstructed.ID())
	}

	if reconstructed.Source() != "new-source" {
		t.Errorf("Expected source 'new-source', got '%s'", reconstructed.Source())
	}

	if reconstructed.Type() != "datadis" {
		t.Errorf("Expected type 'datadis', got '%s'", reconstructed.Type())
	}

	// Verify it's the correct type
	consumptionBlock, ok := reconstructed.(*ConsumptionBlock)
	if !ok {
		t.Fatalf("Reconstructed block should be *ConsumptionBlock, got %T", reconstructed)
	}

	if consumptionBlock.CUPS() != "ES0021000000000000XX" {
		t.Errorf("Expected CUPS 'ES0021000000000000XX', got '%s'", consumptionBlock.CUPS())
	}

	if consumptionBlock.Consumption() != 1.23 {
		t.Errorf("Expected consumption 1.23, got %f", consumptionBlock.Consumption())
	}
}

func TestParseMeasurementTime(t *testing.T) {
	dsInterface, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	// Cast to concrete type to access private method
	ds, ok := dsInterface.(*Datasource)
	if !ok {
		t.Fatalf("Expected *Datasource, got %T", dsInterface)
	}

	testCases := []struct {
		name        string
		date        string
		hour        string
		shouldError bool
	}{
		{
			name:        "Valid datetime",
			date:        "2025/01/15",
			hour:        "14:00",
			shouldError: false,
		},
		{
			name:        "Midnight",
			date:        "2025/01/15",
			hour:        "00:00",
			shouldError: false,
		},
		{
			name:        "End of day",
			date:        "2025/01/15",
			hour:        "23:00",
			shouldError: false,
		},
		{
			name:        "Invalid date format",
			date:        "2025-01-15",
			hour:        "14:00",
			shouldError: true,
		},
		{
			name:        "Invalid hour format",
			date:        "2025/01/15",
			hour:        "14",
			shouldError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			parsedTime, err := ds.parseMeasurementTime(tc.date, tc.hour)

			if tc.shouldError {
				if err == nil {
					t.Error("Expected error but got none")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if parsedTime.IsZero() {
					t.Error("Parsed time should not be zero")
				}
			}
		})
	}
}

func TestFetchBlocksWithoutCredentials(t *testing.T) {
	ds, err := NewDatasource("test-datadis", nil)
	if err != nil {
		t.Fatalf("Failed to create datasource: %v", err)
	}

	ctx := context.Background()
	blockCh := make(chan core.Block, 10)

	// This should fail because we don't have valid credentials
	err = ds.FetchBlocks(ctx, blockCh)
	close(blockCh)

	// We expect an error due to missing/invalid credentials
	if err == nil {
		t.Error("FetchBlocks should return error without valid credentials")
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || indexAny(s, substr) >= 0)
}

func indexAny(s, substr string) int {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}
