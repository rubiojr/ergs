package integration_tests

import (
	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/datasources/testrand"
	"github.com/rubiojr/ergs/pkg/datasources/timestamp"
)

// CreateTestConfig creates a test configuration with multiple local datasources
func CreateTestConfig(tempDir string) *config.Config {
	return &config.Config{
		StorageDir: tempDir,
		Datasources: map[string]config.DatasourceInfo{
			"test_soria_gas": {
				Type: "testrand",
				Config: map[string]interface{}{
					"count":  10,
					"prefix": "SORIA",
					"seed":   12345, // Fixed seed for reproducible tests
				},
			},
			"test_madrid_gas": {
				Type: "testrand",
				Config: map[string]interface{}{
					"count":  15,
					"prefix": "MADRID",
					"seed":   67890, // Different seed for different data
				},
			},
			"test_zaragoza_gas": {
				Type: "timestamp",
				Config: map[string]interface{}{
					"interval_seconds": 60,
				},
			},
		},
	}
}

// CreateTestConfigMinimal creates a minimal test configuration for faster tests
func CreateTestConfigMinimal(tempDir string) *config.Config {
	return &config.Config{
		StorageDir: tempDir,
		Datasources: map[string]config.DatasourceInfo{
			"test_small_soria": {
				Type: "testrand",
				Config: map[string]interface{}{
					"count":  5,
					"prefix": "SORIA",
					"seed":   11111, // Fixed seed for reproducible tests
				},
			},
			"test_small_madrid": {
				Type: "testrand",
				Config: map[string]interface{}{
					"count":  8,
					"prefix": "MADRID",
					"seed":   22222, // Different seed for different data
				},
			},
		},
	}
}

// DatasourceTestCase represents a test case for datasource isolation testing
type DatasourceTestCase struct {
	Name             string
	ExpectedMinCount int
	LocationKeyword  string
}

// GetStandardTestCases returns the standard set of test cases for local test datasources
func GetStandardTestCases() []DatasourceTestCase {
	return []DatasourceTestCase{
		{"test_soria_gas", 5, "SORIA"},    // Expect at least 5 blocks from testrand
		{"test_madrid_gas", 10, "MADRID"}, // Expect at least 10 blocks from testrand
		{"test_zaragoza_gas", 1, ""},      // Expect at least 1 block from timestamp (no location keyword)
	}
}

// GetMinimalTestCases returns minimal test cases for faster testing
func GetMinimalTestCases() []DatasourceTestCase {
	return []DatasourceTestCase{
		{"test_small_soria", 3, "SORIA"},   // Expect at least 3 blocks from testrand
		{"test_small_madrid", 5, "MADRID"}, // Expect at least 5 blocks from testrand
	}
}

// SearchInText performs case-insensitive substring search
func SearchInText(text, searchTerm string) bool {
	if len(searchTerm) == 0 {
		return true
	}
	if len(text) == 0 {
		return false
	}

	// Convert to uppercase for case-insensitive search
	textUpper := toUpper(text)
	searchUpper := toUpper(searchTerm)

	// Simple substring search
	if len(searchUpper) > len(textUpper) {
		return false
	}

	for i := 0; i <= len(textUpper)-len(searchUpper); i++ {
		match := true
		for j := 0; j < len(searchUpper); j++ {
			if textUpper[i+j] != searchUpper[j] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// toUpper converts string to uppercase without using strings package
func toUpper(s string) string {
	result := ""
	for _, r := range s {
		if r >= 'a' && r <= 'z' {
			result += string(r - 32)
		} else {
			result += string(r)
		}
	}
	return result
}

// ContainsLocation checks if text contains the specified location keyword
func ContainsLocation(text, location string) bool {
	return SearchInText(text, location)
}

// CreateDatasourceWithConfig creates a datasource with proper config conversion
func CreateDatasourceWithConfig(registry *core.Registry, instanceName, dsType string, configMap map[string]interface{}) error {
	// First create with nil config (like main application does)
	if err := registry.CreateDatasource(instanceName, dsType, nil); err != nil {
		return err
	}

	// Get the created datasource
	ds, err := registry.GetDatasource(instanceName)
	if err != nil {
		return err
	}

	// For our test datasources, we need to use the actual config types from the datasources
	// Get the proper config type from the datasource and populate it
	configType := ds.ConfigType()

	// Use reflection-like approach to set the config fields
	switch dsType {
	case "testrand":
		return setTestrandConfig(ds, configMap)
	case "timestamp":
		return setTimestampConfig(ds, configMap)
	default:
		// For other datasources, just set the default config
		return ds.SetConfig(configType)
	}
}

// setTestrandConfig sets the testrand config using the actual Config type
func setTestrandConfig(ds core.Datasource, configMap map[string]interface{}) error {
	// Extract values with defaults
	count := 5
	prefix := "RAND"
	seed := int64(12345)

	if c, ok := configMap["count"].(int); ok {
		count = c
	}
	if p, ok := configMap["prefix"].(string); ok {
		prefix = p
	}
	if s, ok := configMap["seed"].(int); ok {
		seed = int64(s)
	}

	// Create the proper config using the imported type
	config := &testrand.Config{
		Count:  count,
		Prefix: prefix,
		Seed:   seed,
	}

	return ds.SetConfig(config)
}

// setTimestampConfig sets the timestamp config using the actual Config type
func setTimestampConfig(ds core.Datasource, configMap map[string]interface{}) error {
	intervalSeconds := 60
	if i, ok := configMap["interval_seconds"].(int); ok {
		intervalSeconds = i
	}

	config := &timestamp.Config{
		IntervalSeconds: intervalSeconds,
	}

	return ds.SetConfig(config)
}
