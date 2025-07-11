package integration_tests

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
)

// CreateTestConfig creates a test configuration with multiple gas station datasources
func CreateTestConfig(tempDir string) *config.Config {
	return &config.Config{
		StorageDir: tempDir,
		Datasources: map[string]config.DatasourceInfo{
			"test_soria_gas": {
				Type: "gasstations",
				Config: map[string]interface{}{
					"latitude":  41.7664, // Soria coordinates
					"longitude": -2.4792, // Soria coordinates
					"radius":    5000.0,  // 5km radius
				},
			},
			"test_madrid_gas": {
				Type: "gasstations",
				Config: map[string]interface{}{
					"latitude":  40.4168, // Madrid coordinates
					"longitude": -3.7038, // Madrid coordinates
					"radius":    5000.0,  // 5km radius
				},
			},
			"test_zaragoza_gas": {
				Type: "gasstations",
				Config: map[string]interface{}{
					"latitude":  41.6488, // Zaragoza coordinates
					"longitude": -0.8891, // Zaragoza coordinates
					"radius":    5000.0,  // 5km radius
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
				Type: "gasstations",
				Config: map[string]interface{}{
					"latitude":  41.7664, // Soria coordinates
					"longitude": -2.4792, // Soria coordinates
					"radius":    2000.0,  // Small 2km radius for fast tests
				},
			},
			"test_small_madrid": {
				Type: "gasstations",
				Config: map[string]interface{}{
					"latitude":  40.4168, // Madrid coordinates
					"longitude": -3.7038, // Madrid coordinates
					"radius":    2000.0,  // Small 2km radius for fast tests
				},
			},
		},
	}
}

// SetupTestEnvironment creates a temporary directory using t.TempDir()
func SetupTestEnvironment(t *testing.T) string {
	return t.TempDir()
}

// VerifyDatabaseFiles checks that expected database files exist
func VerifyDatabaseFiles(t *testing.T, tempDir string, expectedDatabases []string) {
	for _, dbName := range expectedDatabases {
		dbPath := filepath.Join(tempDir, dbName)
		if _, err := os.Stat(dbPath); os.IsNotExist(err) {
			t.Errorf("Database file %s does not exist", dbName)
		}
	}
}

// DatasourceTestCase represents a test case for datasource isolation testing
type DatasourceTestCase struct {
	Name             string
	ExpectedMinCount int
	LocationKeyword  string
}

// GetStandardTestCases returns the standard set of test cases for gas station datasources
func GetStandardTestCases() []DatasourceTestCase {
	return []DatasourceTestCase{
		{"test_soria_gas", 5, "SORIA"},        // Expect at least 5 stations in Soria area
		{"test_madrid_gas", 30, "MADRID"},     // Expect at least 30 stations in Madrid area
		{"test_zaragoza_gas", 20, "ZARAGOZA"}, // Expect at least 20 stations in Zaragoza area
	}
}

// GetMinimalTestCases returns minimal test cases for faster testing
func GetMinimalTestCases() []DatasourceTestCase {
	return []DatasourceTestCase{
		{"test_small_soria", 2, "SORIA"},    // Expect at least 2 stations in small Soria area
		{"test_small_madrid", 10, "MADRID"}, // Expect at least 10 stations in small Madrid area
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

// GetMapKeys returns the keys of a string map as a slice
func GetMapKeys(m map[string]interface{}) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	return keys
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

	// Convert config using the same method as main application
	dsConfig, err := ConvertConfigToType(ds, configMap)
	if err != nil {
		return err
	}

	// Set the converted config
	return ds.SetConfig(dsConfig)
}

// ConvertConfigToType converts raw config to the proper datasource config type
func ConvertConfigToType(ds core.Datasource, rawConfig interface{}) (interface{}, error) {
	configType := ds.ConfigType()

	if rawConfig == nil {
		return configType, nil
	}

	// Use a simple field-by-field copy for map[string]interface{} to struct
	if configMap, ok := rawConfig.(map[string]interface{}); ok {
		return convertMapToStruct(configMap, configType)
	}

	return configType, nil
}

// convertMapToStruct converts a map[string]interface{} to a struct
func convertMapToStruct(configMap map[string]interface{}, target interface{}) (interface{}, error) {
	// For gas stations config, manually create the struct
	if latitude, ok := configMap["latitude"].(float64); ok {
		if longitude, ok := configMap["longitude"].(float64); ok {
			if radius, ok := configMap["radius"].(float64); ok {
				// Create gas station config struct
				return &struct {
					Latitude  float64 `toml:"latitude"`
					Longitude float64 `toml:"longitude"`
					Radius    float64 `toml:"radius"`
				}{
					Latitude:  latitude,
					Longitude: longitude,
					Radius:    radius,
				}, nil
			}
		}
	}

	// If we can't convert, return the target type (default config)
	return target, nil
}
