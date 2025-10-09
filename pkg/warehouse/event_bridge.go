package warehouse

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"sync"
	"syscall"
	"time"
)

// eventBridge is a lightweight, in-process publisher that fans out block events
// to *other processes* (the web/API process) via a Unix domain socket.
// It is intentionally one-way: warehouse -> consumers.
//
// Protocol:
//   - Newline delimited JSON (NDJSON)
//   - Each line is one JSON object
//   - Event types:
//     { "type":"block", "id":"...", "datasource":"...", "ds_type":"...", "created_at":"RFC3339", "text":"...", "metadata":{...} }
//     { "type":"heartbeat", "ts":"RFC3339Nano" }
//     { "type":"info", "message":"..." } (occasionally, e.g. when shutting down)
//     { "type":"error", "message":"...", "detail":"..." } (rare)
//
// Design goals:
//   - Never block ingestion: writes are best-effort; failures are logged silently here.
//   - Multiple consumers may connect simultaneously (fan-out writing to each).
//   - Inbound data from clients is ignored and connections are dropped on read error.
//   - Heartbeats help consumers detect dead connections quickly.
//
// Limits / Non-goals:
//   - No durability / replay. Consumers must do an initial REST backfill on (re)connect.
//   - No per-client backpressure; if a client stalls, its connection eventually errors out.
//   - No authentication (assumes local, permission-controlled socket).
//
// Security considerations:
//   - The Unix socket path should reside in a directory with controlled permissions.
//   - The file is removed and recreated on start, and removed on stop.
//
// Lifecycle:
//   - Created when warehouse.Config.EventSocketPath != "".
//   - Started in Warehouse.Start().
//   - Stopped in Warehouse.Stop() or on process termination.
type eventBridge struct {
	path      string
	ln        net.Listener
	mu        sync.RWMutex
	conns     map[net.Conn]struct{}
	stopCh    chan struct{}
	startOnce sync.Once
	stopOnce  sync.Once
	running   bool
}

// bridgeBlockEvent is the JSON structure emitted for each stored block.
type bridgeBlockEvent struct {
	Type       string         `json:"type"`
	ID         string         `json:"id"`
	Datasource string         `json:"datasource"`
	DSType     string         `json:"ds_type,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	Text       string         `json:"text"`
	Metadata   map[string]any `json:"metadata,omitempty"`
}

// newEventBridge constructs (but does not start) an event bridge.
// If path is empty, the bridge is effectively disabled.
func newEventBridge(path string) *eventBridge {
	return &eventBridge{
		path:   path,
		conns:  make(map[net.Conn]struct{}),
		stopCh: make(chan struct{}),
	}
}

// start initializes the Unix domain socket listener and begins accepting connections.
// Safe to call multiple times; subsequent calls are ignored.
func (b *eventBridge) start() error {
	var err error
	b.startOnce.Do(func() {
		if b.path == "" {
			err = errors.New("event bridge path is empty")
			return
		}

		// Remove stale socket file if it exists
		if st, statErr := os.Stat(b.path); statErr == nil && !st.IsDir() {
			_ = os.Remove(b.path)
		}

		ln, listenErr := net.Listen("unix", b.path)
		if listenErr != nil {
			err = fmt.Errorf("listen on unix socket %s: %w", b.path, listenErr)
			return
		}

		// Set more restrictive permissions (owner RWX, group RWX, others none) for the socket path's parent dir if possible.
		// The socket itself typically inherits process umask; users can manage directory perms externally.
		// Best effort; ignore errors.
		_ = os.Chmod(b.path, 0660)

		b.ln = ln
		b.running = true

		go b.acceptLoop()
		go b.heartbeatLoop()
	})
	return err
}

// acceptLoop continuously accepts new client connections until stopped.
func (b *eventBridge) acceptLoop() {
	for {
		conn, err := b.ln.Accept()
		if err != nil {
			// Check if we are shutting down
			select {
			case <-b.stopCh:
				return
			default:
			}

			// Transient error (e.g., EMFILE)
			if ne, ok := err.(net.Error); ok && ne.Temporary() {
				time.Sleep(50 * time.Millisecond)
				continue
			}

			// Non-temporary error: likely listener closed
			return
		}

		b.mu.Lock()
		b.conns[conn] = struct{}{}
		b.mu.Unlock()

		go b.drain(conn)
	}
}

// drain consumes (and ignores) any inbound data from a client.
// When the client disconnects or an error occurs, the connection is removed.
func (b *eventBridge) drain(c net.Conn) {
	sc := bufio.NewScanner(c)
	for sc.Scan() {
		// Ignore inbound lines (protocol is one-way).
	}
	// Remove connection
	b.mu.Lock()
	delete(b.conns, c)
	b.mu.Unlock()
	_ = c.Close()
}

// heartbeatLoop emits periodic heartbeat frames so consumers can detect stale connections.
func (b *eventBridge) heartbeatLoop() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-b.stopCh:
			return
		case now := <-ticker.C:
			_ = b.broadcast(map[string]any{
				"type": "heartbeat",
				"ts":   now.UTC().Format(time.RFC3339Nano),
			})
		}
	}
}

// publishBlock sends a best-effort block event to all connected consumers.
// It returns immediately; write errors remove the failing connection.
func (b *eventBridge) publishBlock(id, datasource, dsType string, created time.Time, text string, metadata map[string]any) {
	if !b.running {
		return
	}
	evt := bridgeBlockEvent{
		Type:       "block",
		ID:         id,
		Datasource: datasource,
		DSType:     dsType,
		CreatedAt:  created,
		Text:       text,
		Metadata:   metadata,
	}
	_ = b.broadcast(evt) // Best effort
}

// broadcast marshals v to JSON, appends newline, and writes to every connection.
// Dead or slow connections are closed and removed.
func (b *eventBridge) broadcast(v any) error {
	if !b.running {
		return nil
	}

	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	data = append(data, '\n')

	b.mu.Lock()
	defer b.mu.Unlock()

	for c := range b.conns {
		// Small write deadline to avoid blocking ingestion
		_ = c.SetWriteDeadline(time.Now().Add(2 * time.Second))
		if _, werr := c.Write(data); werr != nil {
			// If write error, close and remove connection
			_ = c.Close()
			delete(b.conns, c)
		} else {
			// Clear deadline
			_ = c.SetWriteDeadline(time.Time{})
		}
	}
	return nil
}

// stop shuts down the bridge, closes all connections, and removes the socket file.
// Safe to call multiple times.
func (b *eventBridge) stop() {
	b.stopOnce.Do(func() {
		close(b.stopCh)

		if b.ln != nil {
			_ = b.ln.Close()
		}

		b.mu.Lock()
		for c := range b.conns {
			_ = c.Close()
		}
		b.conns = make(map[net.Conn]struct{})
		b.mu.Unlock()

		// Attempt to remove the socket file
		if b.path != "" {
			_ = os.Remove(b.path)
		}

		b.running = false
	})
}

// isAddrInUseError returns true if the error indicates the address/socket is already in use.
func isAddrInUseError(err error) bool {
	if err == nil {
		return false
	}
	var errno syscall.Errno
	if errors.As(err, &errno) {
		return errno == syscall.EADDRINUSE
	}
	return false
}
