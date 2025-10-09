package cmd

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net"
	"sync"
	"time"

	"github.com/rubiojr/ergs/pkg/realtime"
)

// BridgeBlockEvent represents the JSON payload emitted by the warehouse event bridge
// for each newly stored block. It mirrors the structure produced in
// pkg/warehouse/event_bridge.go.
type BridgeBlockEvent struct {
	Type       string                 `json:"type"`
	ID         string                 `json:"id"`
	Datasource string                 `json:"datasource"`
	DSType     string                 `json:"ds_type"` // Added to allow renderer selection without DB lookup
	CreatedAt  time.Time              `json:"created_at"`
	Text       string                 `json:"text"`
	Metadata   map[string]interface{} `json:"metadata,omitempty"`
}

// Removed local InternalEvent (now using realtime.InternalEvent)

// BridgeConsumer connects to the Unix domain socket exposed by the warehouse
// and streams block events into an internal channel for downstream consumers
// (e.g., a WebSocket hub).
type BridgeConsumer struct {
	socketPath string

	// outCh delivers parsed block events (non-blocking best-effort).
	outCh chan realtime.InternalEvent

	// stop/cancellation
	cancel context.CancelFunc
	ctx    context.Context

	// lifecycle
	startOnce sync.Once
	stopOnce  sync.Once

	// dialing / backoff
	initialBackoff time.Duration
	maxBackoff     time.Duration
}

// NewBridgeConsumer constructs a new consumer.
//
// socketPath: path to the Unix domain socket (empty disables the consumer)
// buffer: size of the outbound channel buffer (recommend >= number of connected websocket listeners * average burst size)
func NewBridgeConsumer(socketPath string, buffer int) *BridgeConsumer {
	if buffer <= 0 {
		buffer = 64
	}
	ctx, cancel := context.WithCancel(context.Background())
	return struct {
		*BridgeConsumer
	}{
		&BridgeConsumer{
			socketPath:     socketPath,
			outCh:          make(chan realtime.InternalEvent, buffer),
			ctx:            ctx,
			cancel:         cancel,
			initialBackoff: 1 * time.Second,
			maxBackoff:     30 * time.Second,
		},
	}.BridgeConsumer
}

// Start launches the background reader and reconnect loop.
// Safe to call multiple times; subsequent calls are ignored.
func (c *BridgeConsumer) Start() {
	c.startOnce.Do(func() {
		if c.socketPath == "" {
			log.Printf("bridge consumer: disabled (no socket path configured)")
			return
		}
		go c.run()
	})
}

// Stop terminates the consumer and closes its output channel.
// Safe to call multiple times.
func (c *BridgeConsumer) Stop() {
	c.stopOnce.Do(func() {
		c.cancel()
		// Give run loop a moment to exit cleanly before closing channel.
		go func() {
			// Small delay to ensure all in-flight sends complete.
			time.Sleep(50 * time.Millisecond)
			close(c.outCh)
		}()
	})
}

// Events returns a receive-only channel for internal events.
// Channel is closed after Stop() and reader loop exits.
func (c *BridgeConsumer) Events() <-chan realtime.InternalEvent {
	return c.outCh
}

// run manages reconnection attempts and a read loop for each established connection.
func (c *BridgeConsumer) run() {
	backoff := c.initialBackoff
	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		conn, err := net.Dial("unix", c.socketPath)
		if err != nil {
			if !errors.Is(err, context.Canceled) {
				log.Printf("bridge consumer: connect failed (%v), retrying in %s", err, backoff)
			}
			select {
			case <-time.After(backoff):
			case <-c.ctx.Done():
				return
			}
			if backoff < c.maxBackoff {
				backoff *= 2
				if backoff > c.maxBackoff {
					backoff = c.maxBackoff
				}
			}
			continue
		}

		log.Printf("bridge consumer: connected to %s", c.socketPath)
		backoff = c.initialBackoff // reset on success
		c.readLoop(conn)

		_ = conn.Close()
		log.Printf("bridge consumer: disconnected")
		// brief pause before immediate reconnect attempt
		select {
		case <-time.After(250 * time.Millisecond):
		case <-c.ctx.Done():
			return
		}
	}
}

// readLoop processes newline-delimited JSON frames until EOF or error.
func (c *BridgeConsumer) readLoop(conn net.Conn) {
	sc := bufio.NewScanner(conn)

	// Increase buffer in case of large metadata payloads.
	const maxLine = 512 * 1024
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, maxLine)

	for sc.Scan() {
		line := sc.Bytes()
		// Fast-path: we can look at a few prefix bytes to discard non-block events cheaply.
		// If it doesn't contain "block" we still parse to determine type when small.
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(line, &raw); err != nil {
			continue // skip malformed line
		}

		tRaw, ok := raw["type"]
		if !ok {
			continue
		}
		var t string
		if err := json.Unmarshal(tRaw, &t); err != nil {
			continue
		}

		if t != "block" {
			// We could handle heartbeat/info/error later if needed.
			continue
		}

		var evt BridgeBlockEvent
		// Reuse raw to decode the full structure:
		if err := json.Unmarshal(line, &evt); err != nil {
			continue
		}

		// Convert bridge event into realtime.BlockEvent + realtime.InternalEvent
		be := realtime.BlockEvent{
			ID:         evt.ID,
			Datasource: evt.Datasource,
			DSType:     evt.DSType,
			CreatedAt:  evt.CreatedAt,
			Text:       evt.Text,
			Metadata:   evt.Metadata,
		}
		re := realtime.InternalEvent{Type: "block", Block: be}
		select {
		case c.outCh <- re:
		default:
			// Drop if downstream is saturated (best-effort semantics).
		}

		select {
		case <-c.ctx.Done():
			return
		default:
		}
	}

	// If scanning ended with error (other than EOF), we can optionally log.
	if err := sc.Err(); err != nil && !errors.Is(err, context.Canceled) {
		// Common benign cases: use of closed network connection when shutting down.
		if !isNetClosed(err) {
			log.Printf("bridge consumer: read error: %v", err)
		}
	}
}

// isNetClosed attempts to identify benign network closure errors.
func isNetClosed(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return msg == "use of closed network connection" || msg == "EOF"
}

// AttachConsumer wires a BridgeConsumer to a realtime hub instance.
// Uses realtime.FirehoseHub directly (legacy hub removed).
func AttachConsumer(ctx context.Context, consumer *BridgeConsumer, hub *realtime.FirehoseHub) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case evt, ok := <-consumer.Events():
				if !ok {
					return
				}
				hub.Broadcast(evt)
			}
		}
	}()
}

// Example integration (pseudo-code):
//
//	hub := realtime.NewFirehoseHub(64)
//	consumer := NewBridgeConsumer(cfg.EventSocketPath, 256)
//	consumer.Start()
//	AttachConsumer(ctx, consumer, hub)
//	// In WebSocket handler: id, ch := hub.Register(); defer hub.Unregister(id); loop reading from ch.
//
// The initial WebSocket connection can still perform a REST backfill (e.g. call
// /api/firehose) to obtain the most recent historical slice, then rely on hub
// events for live updates.
//
// Error handling strategy: best-effort fire-and-forget. If guaranteed delivery
// is required in the future, a persistent queue / sequence table can be added.
func (c *BridgeConsumer) String() string {
	return fmt.Sprintf("BridgeConsumer(socketPath=%s)", c.socketPath)
}
