package integration_tests

import (
	"bytes"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rubiojr/ergs/pkg/config"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/datasources/timestamp"
	"github.com/rubiojr/ergs/pkg/storage"
)

// TestIgnoreStrayDatabase verifies that a stray *.db file that does not
// correspond to any configured datasource (and could have pending migrations)
// does not block creation of the storage manager now that NewManager accepts
// an optional allow list of configured datasources. The manager should:
//   - Ignore the database
//   - Emit a warning mentioning it is being ignored
//   - Allow normal initialization to continue
func TestIgnoreStrayDatabase(t *testing.T) {
	tempDir := t.TempDir()

	// Create a stray database file simulating a removed datasource.
	// We don't need to add any schema; the presence of the file is enough.
	strayDBPath := filepath.Join(tempDir, "removed_datasource.db")
	if err := os.WriteFile(strayDBPath, []byte{}, 0o644); err != nil {
		t.Fatalf("failed to create stray db: %v", err)
	}

	// Create minimal config with only one datasource ("timestamp")
	configPath := filepath.Join(tempDir, "config.toml")
	configContent := `
storage_dir = '` + tempDir + `'

[datasources]
[datasources.timestamp]
type = 'timestamp'
[datasources.timestamp.config]
interval_seconds = 1
`
	if err := os.WriteFile(configPath, []byte(configContent), 0o644); err != nil {
		t.Fatalf("failed to write config: %v", err)
	}

	cfg, err := config.LoadConfig(configPath)
	if err != nil {
		t.Fatalf("failed to load config: %v", err)
	}

	// Prepare registry and register timestamp prototype
	registry := core.NewRegistry()
	t.Cleanup(func() {
		if err := registry.Close(); err != nil {
			t.Logf("warning: closing registry: %v", err)
		}
	})

	if err := registry.RegisterPrototype("timestamp", &timestamp.Datasource{}); err != nil {
		t.Fatalf("failed to register timestamp prototype: %v", err)
	}

	// Create datasource instance
	if err := registry.CreateDatasource("timestamp", "timestamp", nil); err != nil {
		t.Fatalf("failed to create timestamp datasource: %v", err)
	}

	// Set datasource config
	dsCfg := &timestamp.Config{IntervalSeconds: 1}
	ds := registry.GetAllDatasources()["timestamp"]
	if err := ds.SetConfig(dsCfg); err != nil {
		t.Fatalf("failed to set datasource config: %v", err)
	}

	// Capture stdout to verify warning message is emitted
	origStdout := os.Stdout
	readPipe, writePipe, err := os.Pipe()
	if err != nil {
		t.Fatalf("failed to create pipe: %v", err)
	}
	os.Stdout = writePipe
	defer func() {
		_ = writePipe.Close()
		os.Stdout = origStdout
	}()

	// Create manager restricted to configured datasources only
	manager, err := storage.NewManager(cfg.StorageDir, cfg.ListDatasources()...)
	if err != nil {
		t.Fatalf("unexpected error creating manager with stray db present: %v", err)
	}
	t.Cleanup(func() {
		if err := manager.Close(); err != nil {
			t.Logf("warning: closing manager: %v", err)
		}
	})

	// Finish capturing stdout
	_ = writePipe.Close()
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, readPipe); err != nil {
		t.Fatalf("failed to read captured stdout: %v", err)
	}
	captured := buf.String()

	// Assert that a warning about ignoring the stray database was printed
	if !strings.Contains(captured, "ignoring stray database 'removed_datasource.db'") {
		t.Errorf("expected warning about ignoring stray database; got output:\n%s", captured)
	}

	// Manager should allow initializing the configured datasource storage
	if err := manager.InitializeDatasourceStorage("timestamp", ds.Schema()); err != nil {
		t.Fatalf("failed to initialize timestamp storage: %v", err)
	}
}
