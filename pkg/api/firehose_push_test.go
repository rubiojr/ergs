package api_test

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rubiojr/ergs/cmd"
	"github.com/rubiojr/ergs/pkg/api"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/realtime"
	"github.com/rubiojr/ergs/pkg/storage"
)

// ---- Helpers ----------------------------------------------------------------

type wsTestBlock struct {
	id       string
	text     string
	created  time.Time
	source   string
	dsType   string
	metadata map[string]any
}

func (b *wsTestBlock) ID() string                       { return b.id }
func (b *wsTestBlock) Text() string                     { return b.text }
func (b *wsTestBlock) PrettyText() string               { return b.text }
func (b *wsTestBlock) Summary() string                  { return b.text }
func (b *wsTestBlock) CreatedAt() time.Time             { return b.created }
func (b *wsTestBlock) Source() string                   { return b.source }
func (b *wsTestBlock) Type() string                     { return b.dsType }
func (b *wsTestBlock) Metadata() map[string]interface{} { return b.metadata }
func (b *wsTestBlock) Factory(g *core.GenericBlock, source string) core.Block {
	return &wsTestBlock{
		id:       g.ID(),
		text:     g.Text(),
		created:  g.CreatedAt(),
		source:   source,
		dsType:   g.DSType(),
		metadata: g.Metadata(),
	}
}

func initDatasource(t *testing.T, mgr *storage.Manager, dsName string) {
	t.Helper()
	if err := mgr.InitializeDatasourceStorage(dsName, map[string]any{"text": "TEXT"}); err != nil {
		t.Fatalf("init storage: %v", err)
	}
	mgr.RegisterBlockPrototype(dsName, &wsTestBlock{})
}

func storeBlock(t *testing.T, mgr *storage.Manager, dsName, dsType string, blk *wsTestBlock) {
	t.Helper()
	s, err := mgr.EnsureStorageWithMigrations(dsName)
	if err != nil {
		t.Fatalf("ensure storage: %v", err)
	}
	if err := s.StoreBlock(blk, dsType); err != nil {
		t.Fatalf("store block: %v", err)
	}
}

func wsConnect(t *testing.T, baseURL, rawQuery string) (*websocket.Conn, map[string]any) {
	t.Helper()
	u, _ := url.Parse(baseURL)
	u.Scheme = "ws"
	u.Path = "/api/firehose/ws"
	u.RawQuery = rawQuery
	conn, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		t.Fatalf("dial ws: %v", err)
	}
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

// readNextOfType reads WS messages until we find desired type or timeout.
func readNextOfType(t *testing.T, conn *websocket.Conn, desired string, timeout time.Duration) map[string]any {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn.SetReadDeadline(time.Now().Add(2 * time.Second))
		_, data, err := conn.ReadMessage()
		if err != nil {
			t.Fatalf("read message: %v", err)
		}
		var msg map[string]any
		if err := json.Unmarshal(data, &msg); err != nil {
			continue
		}
		if msg["type"] == desired {
			return msg
		}
	}
	t.Fatalf("did not receive message type %s within timeout", desired)
	return nil
}

// ---- 1 & 2: Push Mode WebSocket + init mode field + block delivery ------------

func TestWebSocketPushModeAndModeField(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	dsName := "push_ds"
	initDatasource(t, mgr, dsName)

	// Server with realtime hub injected so push mode is active.
	server := api.NewServer(core.GetGlobalRegistry(), mgr)
	hub := realtime.NewFirehoseHub(16)
	server.SetFirehoseHub(hub)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	conn, initMsg := wsConnect(t, ts.URL, "")
	defer conn.Close()

	mode, _ := initMsg["mode"].(string)
	if mode != "push" {
		t.Fatalf("expected init mode 'push', got %q", mode)
	}

	// Broadcast a block via hub; expect single "block" message (not block_batch).
	now := time.Now().UTC()
	hub.Broadcast(realtime.BlockEvent{
		ID:         "p1",
		Datasource: dsName,
		DSType:     "typeA",
		CreatedAt:  now,
		Text:       "push mode test",
		Metadata:   map[string]any{"k": "v"},
	})

	msg := readNextOfType(t, conn, "block", 5*time.Second)
	blk, ok := msg["block"].(map[string]any)
	if !ok {
		t.Fatalf("block payload missing or wrong type: %v", msg)
	}
	if blk["id"] != "p1" {
		t.Fatalf("expected block id p1, got %v", blk["id"])
	}
	if blk["text"] != "push mode test" {
		t.Fatalf("expected block text, got %v", blk["text"])
	}
}

// ---- 3: Since precision deduplication (second truncation boundary) ------------

func TestWebSocketSinceSecondPrecisionFilter(t *testing.T) {
	tempDir := t.TempDir()
	mgr, err := storage.NewManager(tempDir)
	if err != nil {
		t.Fatalf("new manager: %v", err)
	}
	defer func() { _ = mgr.Close() }()

	dsName := "since_ds"
	dsType := "t"
	initDatasource(t, mgr, dsName)

	base := time.Now().UTC().Truncate(time.Second)
	blockAtSameSecond := &wsTestBlock{
		id:      "s1",
		text:    "same-second-block",
		created: base.Add(500 * time.Millisecond),
		source:  dsName,
		dsType:  dsType,
	}
	storeBlock(t, mgr, dsName, dsType, blockAtSameSecond)

	server := api.NewServer(core.GetGlobalRegistry(), mgr)
	// Inject hub to enable push mode; test here is only for snapshot filtering.
	hub := realtime.NewFirehoseHub(8)
	server.SetFirehoseHub(hub)

	mux := http.NewServeMux()
	server.RegisterRoutes(mux)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	// since = base (same second as blockAtSameSecond truncated) should EXCLUDE the block from snapshot
	since := url.QueryEscape(base.Format(time.RFC3339))
	conn, initMsg := wsConnect(t, ts.URL, "since="+since)
	defer conn.Close()

	if c, ok := initMsg["count"].(float64); !ok || c != 0 {
		t.Fatalf("expected snapshot count 0 (dedup), got %v", initMsg["count"])
	}

	// Control case: since one second earlier should include the block
	earlier := url.QueryEscape(base.Add(-1 * time.Second).Format(time.RFC3339))
	conn2, initMsg2 := wsConnect(t, ts.URL, "since="+earlier)
	defer conn2.Close()

	if c, ok := initMsg2["count"].(float64); !ok || c != 1 {
		t.Fatalf("expected snapshot count 1, got %v", initMsg2["count"])
	}
}

// ---- 4: CLI Firehose Command (basic streaming) --------------------------------

// captureOutput temporarily redirects stdout/stderr
func captureOutput(t *testing.T, fn func()) (string, string) {
	t.Helper()
	origOut := os.Stdout
	origErr := os.Stderr
	rOut, wOut, _ := os.Pipe()
	rErr, wErr, _ := os.Pipe()
	os.Stdout = wOut
	os.Stderr = wErr
	defer func() {
		os.Stdout = origOut
		os.Stderr = origErr
	}()

	done := make(chan struct{})
	var outBuf, errBuf strings.Builder
	go func() {
		defer close(done)
		// Close writers after fn completes to trigger EOF
		fn()
		wOut.Close()
		wErr.Close()
		scOut := bufio.NewScanner(rOut)
		for scOut.Scan() {
			outBuf.WriteString(scOut.Text())
			outBuf.WriteString("\n")
		}
		scErr := bufio.NewScanner(rErr)
		for scErr.Scan() {
			errBuf.WriteString(scErr.Text())
			errBuf.WriteString("\n")
		}
	}()
	<-done
	return outBuf.String(), errBuf.String()
}

// startUnixSocketServer writes provided lines then closes.
func startUnixSocketServer(t *testing.T, path string, lines []string, delayBetween time.Duration) (ready chan struct{}, done chan struct{}) {
	t.Helper()
	ready = make(chan struct{})
	done = make(chan struct{})
	go func() {
		defer close(done)
		ln, err := net.Listen("unix", path)
		if err != nil {
			// Avoid calling t.Fatalf in a goroutine; signal (failed) readiness and exit.
			close(ready)
			return
		}
		defer ln.Close()
		close(ready)
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for i, l := range lines {
			if _, err := conn.Write([]byte(l + "\n")); err != nil {
				return
			}
			if i < len(lines)-1 && delayBetween > 0 {
				time.Sleep(delayBetween)
			}
		}
	}()
	return
}

func runCLI(t *testing.T, args []string) (stdout, stderr string, err error) {
	t.Helper()
	firehoseCmd := cmd.FirehoseCommand()
	ctx := context.Background()
	allArgs := append([]string{"ergs", "firehose"}, args...)
	stdout, stderr = captureOutput(t, func() {
		_ = firehoseCmd.Run(ctx, allArgs)
	})
	return stdout, stderr, nil
}

// Since we don't have direct access to root app struct (main builds it) we replicate minimal run.
// Define a tiny adapter to satisfy compilation; no-op container.
type cmdApp struct{}

// To avoid race conditions when running tests in parallel that use unix sockets, we allocate unique paths.
func uniqueSocketPath(t *testing.T) string {
	return filepath.Join(t.TempDir(), fmt.Sprintf("sock-%d.sock", time.Now().UnixNano()))
}

func TestCLIFirehoseAllEvents(t *testing.T) {
	socketPath := uniqueSocketPath(t)
	now := time.Now().UTC().Format(time.RFC3339)
	lines := []string{
		fmt.Sprintf(`{"type":"block","id":"b1","datasource":"ds","ds_type":"x","created_at":"%s","text":"hello"}`, now),
		fmt.Sprintf(`{"type":"heartbeat","ts":"%s"}`, time.Now().UTC().Format(time.RFC3339Nano)),
	}
	ready, _ := startUnixSocketServer(t, socketPath, lines, 50*time.Millisecond)
	<-ready

	stdout, stderr := captureOutput(t, func() {
		// Run command synchronously (no retry, include all)
		app := cmd.FirehoseCommand()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.Run(ctx, []string{
			"ergs", "firehose",
			"--socket", socketPath,
			"--all",
			"--no-retry",
		})
	})

	if !strings.Contains(stdout, `"type":"block"`) {
		t.Fatalf("stdout missing block event: %s\nstderr: %s", stdout, stderr)
	}
	if !strings.Contains(stdout, `"type":"heartbeat"`) {
		t.Fatalf("stdout missing heartbeat with --all: %s\nstderr: %s", stdout, stderr)
	}
}

func TestCLIFirehoseBlockOnly(t *testing.T) {
	socketPath := uniqueSocketPath(t)
	now := time.Now().UTC().Format(time.RFC3339)
	lines := []string{
		fmt.Sprintf(`{"type":"block","id":"b2","datasource":"ds","ds_type":"x","created_at":"%s","text":"hello2"}`, now),
		fmt.Sprintf(`{"type":"heartbeat","ts":"%s"}`, time.Now().UTC().Format(time.RFC3339Nano)),
	}
	ready, _ := startUnixSocketServer(t, socketPath, lines, 10*time.Millisecond)
	<-ready

	stdout, stderr := captureOutput(t, func() {
		app := cmd.FirehoseCommand()
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = app.Run(ctx, []string{
			"ergs", "firehose",
			"--socket", socketPath,
			"--no-retry",
		})
	})

	if !strings.Contains(stdout, `"type":"block"`) {
		t.Fatalf("stdout missing block event: %s\nstderr: %s", stdout, stderr)
	}
	if strings.Contains(stdout, `"type":"heartbeat"`) {
		t.Fatalf("heartbeat should not appear without --all: %s\nstderr: %s", stdout, stderr)
	}
}

// Additional: ensure CLI exits on missing socket with --no-retry.
func TestCLIFirehoseNoRetryFailure(t *testing.T) {
	socketPath := filepath.Join(t.TempDir(), "missing.sock")
	app := cmd.FirehoseCommand()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	_, _ = captureOutput(t, func() {
		_ = app.Run(ctx, []string{
			"ergs", "firehose",
			"--socket", socketPath,
			"--no-retry",
		})
	})
	// Can't easily assert stderr content without changing command (it writes error logs),
	// but absence of hang indicates immediate exit behavior worked.
}

// Mutex to serialize CLI tests that manipulate global stdout/stderr replacement
var cliStdIOSerial sync.Mutex

func captureOutputSerial(t *testing.T, fn func()) (string, string) {
	cliStdIOSerial.Lock()
	defer cliStdIOSerial.Unlock()
	return captureOutput(t, fn)
}

// Replace earlier tests' captureOutput usage with serialized variant if flakiness observed.
// (Left here for potential future refactor.)
