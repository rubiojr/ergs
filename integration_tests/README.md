# Integration Tests

This directory contains integration tests for the ergs system to ensure that critical functionality works correctly, particularly the isolation between multiple datasource instances.

## Purpose

These tests were created specifically to prevent the cross-contamination bug that was discovered where multiple instances of the same datasource type (e.g., `soria_gas`, `madrid_gas`, `zaragoza_gas`) would accidentally store data in the wrong databases when fetching concurrently.

## Test Files

### `multi_datasource_test.go`
Comprehensive integration tests that verify:
- **Data isolation**: Each datasource instance stores data only in its own database
- **Search functionality**: Searches work correctly within specific datasources
- **Cross-datasource search**: Search across all datasources returns correct results
- **Database schema**: Proper table creation and structure
- **Block source assignment**: Blocks have correct source field matching instance name

### `quick_isolation_test.go`
Fast integration tests designed for CI/CD pipelines:
- **Basic isolation**: Quick verification that datasources don't cross-contaminate
- **Search isolation**: Rapid test of search functionality
- **Type vs Name methods**: Verification that `Type()` and `Name()` return correct values
- **Factory instance naming**: Tests that datasource factories properly handle instance names

### `test_helpers.go`
Helper functions and utilities for tests:
- Test configuration creation
- Common test cases and scenarios
- Utility functions for string searching and validation

## Running Tests

### Prerequisites

Make sure you have Go 1.24+ installed and the required build tags:

```bash
# Install dependencies
make deps
```

### Quick Tests (Recommended for CI/CD)

```bash
# Run quick integration tests (fast, minimal network usage)
make test-integration-quick
```

This runs tests with:
- Small radius searches (2km instead of 5km)
- Timeout of 1 minute
- Minimal data fetching

### Full Integration Tests

```bash
# Run all integration tests
make test-integration
```

This runs comprehensive tests that:
- Use realistic radius sizes (5km)
- Fetch more data for thorough validation
- Take longer to complete (up to 10 minutes)

### All Tests

```bash
# Run unit tests and integration tests
make test-all
```

## Test Environment

### Temporary Directories
All tests use `t.TempDir()` to create isolated temporary directories that are automatically cleaned up when tests complete.

### Network Dependencies
Integration tests require internet access to fetch real gas station data from the Spanish government API. Tests will skip or fail gracefully if network is unavailable.

### Test Data
Tests use real coordinates for Spanish cities:
- **Soria**: 41.7664, -2.4792 (small city, ~10 stations)
- **Madrid**: 40.4168, -3.7038 (large city, ~60+ stations)
- **Zaragoza**: 41.6488, -0.8891 (medium city, ~40+ stations)

## Key Test Scenarios

### 1. Datasource Isolation
```go
// Verifies that blocks from different datasource instances
// don't end up in the wrong database
func TestMultipleDatasourceIsolation(t *testing.T)
```

**What it tests:**
- Each datasource creates its own `.db` file
- Blocks have correct `source` field matching instance name
- No cross-contamination between Madrid, Soria, and Zaragoza data

### 2. Search Functionality
```go
// Verifies search works correctly within specific datasources
// and across multiple datasources
```

**What it tests:**
- Searching within specific datasource returns only that datasource's data
- Cross-datasource search aggregates results correctly
- Search results have correct source attribution

### 3. Type vs Name Methods
```go
// Verifies datasource Type() and Name() methods work correctly
func TestDatasourceTypeVsInstanceName(t *testing.T)
```

**What it tests:**
- `ds.Type()` returns datasource type (e.g., "gasstations")
- `ds.Name()` returns instance name (e.g., "soria_gas")
- Multiple instances of same type have different names

### 4. Block Source Matching
```go
// Verifies blocks created by datasources have correct source field
func TestBlockSourceMatching(t *testing.T)
```

**What it tests:**
- Blocks created by "soria_gas" have `source = "soria_gas"`
- Blocks created by "madrid_gas" have `source = "madrid_gas"`
- No blocks have generic source like "gasstations"

## Continuous Integration

### GitHub Actions
The CI pipeline runs tests in stages:

1. **Unit Tests**: Fast tests without network dependencies
2. **Quick Integration Tests**: Fast integration tests with small datasets
3. **Full Integration Tests**: Only on main branch pushes

### Test Timeouts
- Quick tests: 5 minutes timeout
- Full tests: 10 minutes timeout

### Failure Scenarios
Tests will fail if:
- Cross-contamination is detected (blocks in wrong database)
- Search returns results from wrong datasource
- Database files are not created properly
- Network timeouts occur during data fetching

## Adding New Tests

When adding new datasource types or modifying existing ones:

1. **Add test cases** to verify isolation
2. **Update helper functions** if needed
3. **Test both Type() and Name()** methods
4. **Verify block source assignment** is correct
5. **Add to CI pipeline** if appropriate

### Example Test Case
```go
func TestNewDatasourceIsolation(t *testing.T) {
    tempDir := t.TempDir()
    
    // Create multiple instances of new datasource type
    // Fetch data concurrently
    // Verify no cross-contamination
    // Test search functionality
}
```

## Debugging Test Failures

### Common Issues

1. **Network timeouts**: Tests may fail if government API is slow
   - Solution: Run tests again or increase timeout
   
2. **Cross-contamination detected**: Blocks found in wrong database
   - This indicates a serious bug in datasource isolation
   - Check warehouse `storeBlock()` logic
   - Verify block source assignment
   
3. **Search returning wrong results**: Results from unrelated datasources
   - Check storage manager search logic
   - Verify FTS table integrity

### Debug Logging
Add debug logging to tests:
```go
t.Logf("Database %s has %d blocks", datasourceName, len(blocks))
for _, block := range blocks {
    t.Logf("Block source: %s, expected: %s", block.Source(), expectedSource)
}
```

## Performance Considerations

### Test Speed
- Quick tests use small radius (2km) for faster execution
- Full tests use realistic radius (5km) for thorough validation
- Tests run in parallel when possible

### Resource Usage
- Each test creates temporary databases
- Network bandwidth used for API calls
- Memory usage scales with number of concurrent datasources

## Historical Context

These tests were created after discovering a critical bug where multiple gas station datasources (`soria_gas`, `madrid_gas`, `zaragoza_gas`) were cross-contaminating each other's data during concurrent fetching. The bug was caused by:

1. All gas station blocks had `source = "gasstations"` (datasource type)
2. Warehouse `storeBlock()` method used `ds.Name() == block.Source()` lookup
3. All gas station datasources had `ds.Name() = "gasstations"`
4. First matching datasource in iteration order received all blocks

The fix involved:
1. Adding `Type()` method for datasource type
2. Using `Name()` method for instance name
3. Setting block source to instance name instead of type name
4. Simplifying warehouse storage logic

These integration tests ensure this type of bug cannot regress.