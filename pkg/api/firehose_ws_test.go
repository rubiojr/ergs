package api

import (
	"context"
	"encoding/json"

	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
	"github.com/rubiojr/ergs/pkg/warehouse"
)

// testBlock implements core.Block for testing.
type testBlock struct {
	id       string
	text     string
	created  time.Time
	source   string
	dsType   string
	metadata map[string]interface{}
}

func (b *testBlock) ID() string                       { return b.id }
func (b *testBlock) Text() string                     { return b.text }
func (b *testBlock) CreatedAt() time.Time             { return b.created }
func (b *testBlock) Source() string                   { return b.source }
func (b *testBlock) Type() string                     { return b.dsType }
func (b *testBlock) Metadata() map[string]interface{} { return b.metadata }
func (b *testBlock) PrettyText() string               { return b.text }
func (b *testBlock) Summary() string                  { return b.text }
func (b *testBlock) Factory(g *core.GenericBlock, s string) core.Block {
	return &testBlock{
		id:       g.ID(),
		text:     g.Text(),
		created:  g.CreatedAt(),
		source:   s,
		dsType:   g.DSType(),
		metadata: g.Metadata(),
	}
}

// helper to store a block
func storeTestBlock(t *testing.T, mgr *storage.Manager, dsName, dsType string, blk *testBlock) {
	t.Helper()
	gs, err := mgr.EnsureStorageWithMigrations(dsName)
	if err != nil {
		t.Fatalf("ensure storage: %v", err)
	}
	if err := gs.StoreBlock(blk, dsType); err != nil {
		t.Fatalf("store block: %v", err)
	}
}

func newServerWithData(t *testing.T, dsName string, blocks []*testBlock) (*Server, *storage.Manager) {
	t.Helper()
	tmpDir := t.TempDir()

	mgr, err := storage.NewManager(tmpDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	// Initialize schema (minimal)
	if err := mgr.InitializeDatasourceStorage(dsName, map[string]any{"text": "TEXT"}); err != nil {
		t.Fatalf("init storage: %v", err)
	}

	// Register block prototype (generic)
	mgr.RegisterBlockPrototype(dsName, &testBlock{})

	// Store blocks
	for _, b := range blocks {
		storeTestBlock(t, mgr, dsName, b.dsType, b)
	}

	srv := NewServer(core.GetGlobalRegistry(), mgr)
	return srv, mgr
}

func wsDial(t *testing.T, ts *httptest.Server, rawQuery string) (*websocket.Conn, map[string]any) {
	t.Helper()
	u, _ := url.Parse(ts.URL)
	u.Scheme = "ws"
	u.Path = "/api/firehose/ws"
	u.RawQuery = rawQuery

	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}

	// Read init message
	_, data, err := conn.ReadMessage()
	if err != nil {
		t.Fatalf("read init: %v", err)
	}

	var msg map[string]any
	if err := json.Unmarshal(data, &msg); err != nil {
		t.Fatalf("unmarshal init: %v", err)
	}
	if msg["type"] != "init" {
		t.Fatalf("expected init message, got %v", msg["type"])
	}
	return conn, msg
}

func extractBlockIDs(t *testing.T, initMsg map[string]any) []string {
	t.Helper()
	rawBlocks, ok := initMsg["blocks"].([]interface{})
	if !ok {
		return nil
	}
	var ids []string
	for _, rb := range rawBlocks {
		if m, ok := rb.(map[string]interface{}); ok {
			if id, ok := m["id"].(string); ok {
				ids = append(ids, id)
			}
		}
	}
	return ids
}

func TestWebSocketFirehoseSinceParameter(t *testing.T) {
	dsName := "ws_test_ds"
	dsType := "test_type"

	now := time.Now().UTC()
	blk1 := &testBlock{
		id:       "b1",
		text:     "first block",
		created:  now.Add(-3 * time.Minute),
		source:   dsName,
		dsType:   dsType,
		metadata: map[string]interface{}{"k": "v1"},
	}
	blk2 := &testBlock{
		id:       "b2",
		text:     "second block",
		created:  now.Add(-1 * time.Minute),
		source:   dsName,
		dsType:   dsType,
		metadata: map[string]interface{}{"k": "v2"},
	}

	server, mgr := newServerWithData(t, dsName, []*testBlock{blk1, blk2})

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	t.Run("baseline init without since returns both blocks", func(t *testing.T) {
		conn, initMsg := wsDial(t, ts, "")
		defer func() { _ = conn.Close() }()

		ids := extractBlockIDs(t, initMsg)
		if len(ids) == 0 {
			t.Fatalf("expected some blocks, got none")
		}
		// Both in descending order (latest first) is acceptable but we just check membership
		found := map[string]bool{}
		for _, id := range ids {
			found[id] = true
		}
		if !found["b1"] || !found["b2"] {
			t.Fatalf("expected both b1 and b2, got %v", ids)
		}
	})

	t.Run("since newer than all blocks returns empty snapshot", func(t *testing.T) {
		since := url.QueryEscape(blk2.CreatedAt().Add(30 * time.Second).Format(time.RFC3339))
		conn, initMsg := wsDial(t, ts, "since="+since)
		defer func() { _ = conn.Close() }()

		if c, ok := initMsg["count"].(float64); !ok || c != 0 {
			t.Fatalf("expected count 0, got %v", initMsg["count"])
		}
		ids := extractBlockIDs(t, initMsg)
		if len(ids) != 0 {
			t.Fatalf("expected no block IDs, got %v", ids)
		}
	})

	t.Run("since between blk1 and blk2 returns only blk2", func(t *testing.T) {
		cursor := blk1.CreatedAt().Add(30 * time.Second) // after blk1, before blk2
		conn, initMsg := wsDial(t, ts, "since="+url.QueryEscape(cursor.Format(time.RFC3339)))
		defer func() { _ = conn.Close() }()

		ids := extractBlockIDs(t, initMsg)
		if len(ids) != 1 || ids[0] != "b2" {
			t.Fatalf("expected only b2, got %v", ids)
		}
	})

	t.Run("poll fallback yields only new blocks after since", func(t *testing.T) {
		// Simulate absence of realtime hub by not injecting one (already true).
		// Connect with since at blk2 time so snapshot empty.
		since := url.QueryEscape(blk2.CreatedAt().Format(time.RFC3339))
		conn, initMsg := wsDial(t, ts, "since="+since)
		defer func() { _ = conn.Close() }()

		if c, _ := initMsg["count"].(float64); c > 1 {
			t.Fatalf("expected at most one block in init (second-precision since filtering), got count=%v", c)
		}
		if c, _ := initMsg["count"].(float64); c == 1 {
			ids := extractBlockIDs(t, initMsg)
			if len(ids) != 1 || ids[0] != "b2" {
				t.Fatalf("unexpected block(s) in init after since filter: %v", ids)
			}
		}

		// Store a new block after connection; polling loop (every 5s) should pick it up.
		newBlock := &testBlock{
			id:       "b3",
			text:     "third block",
			created:  time.Now().UTC().Add(2 * time.Second),
			source:   dsName,
			dsType:   dsType,
			metadata: map[string]interface{}{"k": "v3"},
		}
		storeTestBlock(t, mgr, dsName, dsType, newBlock)

		readWithTimeout := func() (map[string]any, error) {
			_ = conn.SetReadDeadline(time.Now().Add(8 * time.Second)) // > poll interval
			_, data, err := conn.ReadMessage()
			if err != nil {
				return nil, err
			}
			var msg map[string]any
			if err := json.Unmarshal(data, &msg); err != nil {
				return nil, err
			}
			return msg, nil
		}

		var got map[string]any
		var err error
		got, err = readWithTimeout()
		if err != nil {
			t.Fatalf("waiting for poll batch: %v", err)
		}
		if got["type"] != "block_batch" {
			// Could receive heartbeat first; keep reading until batch or timeout
			deadline := time.Now().Add(10 * time.Second)
			for time.Now().Before(deadline) && got["type"] != "block_batch" {
				got, err = readWithTimeout()
				if err != nil {
					t.Fatalf("reading subsequent message: %v", err)
				}
			}
		}
		if got["type"] != "block_batch" {
			t.Fatalf("expected block_batch, got %v", got["type"])
		}
		rawBlocks, _ := got["blocks"].([]interface{})
		found := false
		for _, rb := range rawBlocks {
			if m, ok := rb.(map[string]interface{}); ok {
				if m["id"] == "b3" {
					found = true
				}
			}
		}
		if !found {
			t.Fatalf("expected new block b3 in batch, got %v", rawBlocks)
		}
	})
}

func TestSinceParsingValidation(t *testing.T) {
	// Ensure ParseSearchParams rejects invalid since values.
	values := map[string][]string{
		"since": {"not-a-timestamp"},
	}
	_, err := storage.ParseSearchParams(values)
	if err == nil {
		t.Fatal("expected error for invalid since timestamp, got nil")
	}

	// Accept valid RFC3339
	ts := time.Now().UTC().Format(time.RFC3339)
	values = map[string][]string{
		"since": {ts},
	}
	params, err := storage.ParseSearchParams(values)
	if err != nil {
		t.Fatalf("unexpected error parsing valid since: %v", err)
	}
	if params.Since == nil || params.Since.Format(time.RFC3339) != ts {
		t.Fatalf("since not parsed correctly: %#v", params.Since)
	}
}

func TestSinceOverridesStartDate(t *testing.T) {
	ts := time.Now().UTC().Add(-1 * time.Hour).Format(time.RFC3339)
	startDate := time.Now().UTC().Add(-24 * time.Hour).Format("2006-01-02")
	values := map[string][]string{
		"since":      {ts},
		"start_date": {startDate},
	}
	params, err := storage.ParseSearchParams(values)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if params.Since == nil {
		t.Fatalf("expected since parsed")
	}
	if params.StartDate != nil {
		t.Fatalf("expected StartDate cleared when since provided, got %v", params.StartDate)
	}
}

// Sanity check: ensure temp dir isolation for multiple test runs
func TestTempDirIsolation(t *testing.T) {
	dir1 := t.TempDir()
	dir2 := t.TempDir()
	if dir1 == dir2 {
		t.Fatalf("expected different temp dirs")
	}
	if filepath.Dir(dir1) == "" || filepath.Dir(dir2) == "" {
		t.Fatalf("unexpected empty temp dir base")
	}
	// removed dummy fmt usage; fmt import no longer needed
}

// dummyDatasource is a minimal datasource used to verify the event bridge socket creation.
type dummyDatasource struct{}

var _ core.Datasource = (*dummyDatasource)(nil)

func (d *dummyDatasource) Type() string                    { return "dummy" }
func (d *dummyDatasource) Name() string                    { return "dummy" }
func (d *dummyDatasource) Schema() map[string]any          { return map[string]any{"text": "TEXT"} }
func (d *dummyDatasource) BlockPrototype() core.Block      { return &testBlock{} }
func (d *dummyDatasource) ConfigType() interface{}         { return nil }
func (d *dummyDatasource) SetConfig(cfg interface{}) error { return nil }
func (d *dummyDatasource) GetConfig() interface{}          { return nil }
func (d *dummyDatasource) Close() error                    { return nil }
func (d *dummyDatasource) Factory(instanceName string, c interface{}) (core.Datasource, error) {
	return &dummyDatasource{}, nil
}
func (d *dummyDatasource) FetchBlocks(ctx context.Context, ch chan<- core.Block) error {
	return nil
}

// TestWarehouseEventSocketCreatesSocket ensures EventSocketPath triggers socket creation on Start.
func TestWarehouseEventSocketCreatesSocket(t *testing.T) {
	tmpDir := t.TempDir()
	socketPath := filepath.Join(tmpDir, "bridge.sock")

	mgr, err := storage.NewManager(tmpDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}

	cfg := warehouse.Config{
		OptimizeInterval: 0,
		EventSocketPath:  socketPath,
	}
	wh := warehouse.NewWarehouse(cfg, mgr)

	if err := wh.AddDatasourceWithInterval("dummy", &dummyDatasource{}, 0); err != nil {
		t.Fatalf("add datasource: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := wh.Start(ctx); err != nil {
		t.Fatalf("start warehouse: %v", err)
	}
	defer wh.Stop()

	if stat, err := os.Stat(socketPath); err != nil || stat.IsDir() {
		t.Fatalf("expected socket file %s to exist (file, not dir): %v", socketPath, err)
	}
}
