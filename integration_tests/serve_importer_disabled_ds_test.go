package integration_tests

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"

	"github.com/urfave/cli/v3"

	"github.com/rubiojr/ergs/cmd"
	_ "github.com/rubiojr/ergs/pkg/datasources/importer"
	_ "github.com/rubiojr/ergs/pkg/datasources/timestamp"
)

// TestServeImporterDropsUnknownDatasource
//
// This version invokes the actual ServeCommand (CLI path) to avoid duplicating
// service bootstrap logic. It launches the command in a goroutine and then
// sends SIGINT to terminate after allowing the initial fetch to run.
//
// Scenario:
//
//	Configured datasources: timestamp, importer
//	Importer returns:
//	  - block for timestamp   (enabled -> should persist, timestamp.db created)
//	  - block for firefox     (disabled -> should be dropped, firefox.db absent)
//
// Assertions:
//   - timestamp.db exists
//   - firefox.db does NOT exist
func TestServeImporterDropsUnknownDatasource(t *testing.T) {
	// Start mock importer server
	ts := newMockImporterServer(t)
	defer ts.Close()

	// Temp workspace + config file
	workDir := t.TempDir()
	storageDir := filepath.Join(workDir, "storage")
	if err := os.MkdirAll(storageDir, 0o755); err != nil {
		t.Fatalf("mkdir storage: %v", err)
	}
	cfgPath := writeImporterServeConfig(t, workDir, storageDir, ts.URL)

	// Build a mini CLI app with only the serve command
	app := &cli.Command{
		Name:  "ergs-test",
		Usage: "test harness",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:  "config",
				Usage: "Configuration file path",
				Value: cfgPath,
			},
			&cli.BoolFlag{
				Name:  "debug",
				Usage: "Enable debug logging",
				Value: false,
			},
		},
		Commands: []*cli.Command{
			cmd.ServeCommand(),
		},
	}

	var stderr bytes.Buffer
	log.SetOutput(&stderr) // capture log output (serve uses log package)

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()

	// Run CLI (serve) in goroutine
	var runErr error
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		args := []string{"ergs-test", "--config", cfgPath, "serve"}
		runErr = app.Run(ctx, args)
	}()

	// Allow time for initial fetch: importer + timestamp
	time.Sleep(5 * time.Second)

	// Send SIGINT to trigger graceful shutdown (serve listens for it)
	if err := syscall.Kill(syscall.Getpid(), syscall.SIGINT); err != nil {
		t.Fatalf("failed to send SIGINT: %v", err)
	}

	// Wait for serve to exit
	wg.Wait()

	// (Serve returns nil on clean shutdown; if interrupted mid-process it might still be nil)
	if runErr != nil {
		t.Fatalf("serve returned error: %v\nSTDERR:\n%s", runErr, stderr.String())
	}

	// Assertions
	timestampDB := filepath.Join(storageDir, "timestamp.db")
	firefoxDB := filepath.Join(storageDir, "firefox.db")

	if _, err := os.Stat(timestampDB); err != nil {
		t.Fatalf("expected timestamp.db to exist, got error: %v\nLogs:\n%s", err, stderr.String())
	}
	if _, err := os.Stat(firefoxDB); err == nil {
		t.Fatalf("firefox.db should NOT exist (disabled datasource)\nLogs:\n%s", stderr.String())
	}
}

// (legacy helper removed – using ServeCommand path now)

// (unused config conversion helper removed)

// writeImporterServeConfig writes a config enabling timestamp and importer only.
func writeImporterServeConfig(t *testing.T, workDir, storageDir, importerURL string) string {
	t.Helper()
	cfg := fmt.Sprintf(`
storage_dir = %q
fetch_interval = "30m"

[datasources.timestamp]
type = "timestamp"

[datasources.importer]
type = "importer"
  [datasources.importer.config]
  api_url = %q
`, storageDir, importerURL)

	cfgPath := filepath.Join(workDir, "config.toml")
	if err := os.WriteFile(cfgPath, []byte(strings.TrimSpace(cfg)+"\n"), 0o644); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return cfgPath
}

// mockImporterServer returns blocks for timestamp (enabled) and firefox (disabled)
func newMockImporterServer(t *testing.T) *httptest.Server {
	t.Helper()

	mux := http.NewServeMux()
	mux.HandleFunc("/api/blocks/export", func(w http.ResponseWriter, r *http.Request) {
		type blk struct {
			ID         string         `json:"id"`
			Text       string         `json:"text"`
			CreatedAt  time.Time      `json:"created_at"`
			Type       string         `json:"type"`
			Datasource string         `json:"datasource"`
			Metadata   map[string]any `json:"metadata"`
		}
		resp := struct {
			Blocks []blk `json:"blocks"`
			Count  int   `json:"count"`
		}{
			Blocks: []blk{
				{
					ID:         "enabled-1",
					Text:       "Enabled timestamp block",
					CreatedAt:  time.Now().UTC(),
					Type:       "note",
					Datasource: "timestamp",
					Metadata:   map[string]any{},
				},
				{
					ID:         "disabled-1",
					Text:       "Disabled firefox block",
					CreatedAt:  time.Now().UTC(),
					Type:       "note",
					Datasource: "firefox",
					Metadata:   map[string]any{},
				},
			},
			Count: 2,
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(resp)
	})

	return httptest.NewServer(mux)
}
