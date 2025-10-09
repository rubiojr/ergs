package realtime

// Package realtime provides shared, transport-agnostic types and a lightweight
// in‑process publish/subscribe hub used to fan out realtime block events to
// multiple listeners (e.g. WebSocket sessions).
//
// Motivation:
//
// Previously, the realtime hub + event envelope lived in the cmd layer which
// prevented the API (pkg/api) layer from type‑asserting hub subscriptions
// cleanly (duplicate "InternalEvent" types in different packages caused the
// WebSocket firehose to always fall back to polling). Centralizing these
// definitions here eliminates import cycles and enables true push delivery.
//
// Design Goals:
//   - Zero external dependencies beyond the standard library.
//   - Best‑effort fan‑out: slow listeners drop events (never backpressure ingestion).
//   - No persistence or replay semantics (ephemeral stream).
//   - Simple, extensible event envelope (currently only block events).
//
// If durable or replayable semantics are needed in the future, this package
// becomes the seam where a broker (Redis Streams, NATS, Kafka, etc.) can be
// introduced behind a compatible interface.

import (
	"sync"
	"time"
)

// BlockEvent represents a single ingested block delivered over the realtime
// bridge / hub path. It intentionally mirrors a subset of the stored block
// fields plus renderer‑relevant metadata.
//
// Fields:
//   - ID:          Unique block identifier (scoped to datasource instance).
//   - Datasource:  Datasource instance name (block.Source()).
//   - DSType:      Datasource *type* (e.g. "github", "hackernews") used by renderers.
//   - CreatedAt:   Original creation time (UTC recommended).
//   - Text:        Raw searchable text (same content indexed in FTS).
//   - Metadata:    Arbitrary datasource‑specific metadata (non-durable across protocol versions).
type BlockEvent struct {
	ID         string         `json:"id"`
	Datasource string         `json:"datasource"`
	DSType     string         `json:"ds_type"`
	CreatedAt  time.Time      `json:"created_at"`
	Text       string         `json:"text"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	// Future extension points:
	//   ProtocolVersion int    `json:"proto_version,omitempty"`
	//   Sequence        uint64 `json:"seq,omitempty"`
}

// InternalEvent is the hub's internal envelope allowing future introduction
// of additional event kinds (heartbeat, info, etc.) without changing channel
// element types. For now only Type == "block" is produced.
type InternalEvent struct {
	Type  string     `json:"type"`
	Block BlockEvent `json:"block"`
}

// FirehoseHub is an in‑memory fan‑out dispatcher. Each registered listener
// receives events via its own buffered channel. If a listener's channel buffer
// is full when an event arrives, that event is *dropped for that listener only*.
// This prevents a single slow consumer from degrading overall ingestion / delivery.
//
// The hub is concurrency‑safe.
type FirehoseHub struct {
	mu        sync.RWMutex
	listeners map[uint64]chan InternalEvent
	nextID    uint64
	bufSize   int
}

// NewFirehoseHub constructs a new hub with per-listener buffer size.
// If bufSize <= 0, a default of 32 is used.
func NewFirehoseHub(bufSize int) *FirehoseHub {
	if bufSize <= 0 {
		bufSize = 32
	}
	return &FirehoseHub{
		listeners: make(map[uint64]chan InternalEvent),
		bufSize:   bufSize,
	}
}

// Register adds a new listener and returns (listenerID, receiveOnlyChannel).
// Callers must later Unregister(id) to release resources.
func (h *FirehoseHub) Register() (uint64, <-chan InternalEvent) {
	h.mu.Lock()
	defer h.mu.Unlock()
	id := h.nextID
	h.nextID++
	ch := make(chan InternalEvent, h.bufSize)
	h.listeners[id] = ch
	return id, ch
}

// Unregister removes the listener with the given id and closes its channel.
// It is safe to call multiple times; unknown ids are ignored.
func (h *FirehoseHub) Unregister(id uint64) {
	h.mu.Lock()
	defer h.mu.Unlock()
	if ch, ok := h.listeners[id]; ok {
		delete(h.listeners, id)
		close(ch)
	}
}

// Broadcast delivers an event to all registered listeners (best effort).
// Accepted input types:
//   - InternalEvent
//   - BlockEvent (will be wrapped as InternalEvent{Type:"block"})
//
// Any other type is ignored silently.
func (h *FirehoseHub) Broadcast(event interface{}) {
	var ie InternalEvent
	switch v := event.(type) {
	case InternalEvent:
		ie = v
	case BlockEvent:
		ie = InternalEvent{Type: "block", Block: v}
	default:
		return
	}

	h.mu.RLock()
	defer h.mu.RUnlock()
	for _, ch := range h.listeners {
		select {
		case ch <- ie:
		default:
			// Drop for slow listener.
		}
	}
}

// Size returns the current number of active listeners (approximate).
func (h *FirehoseHub) Size() int {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return len(h.listeners)
}

// Convenience helpers --------------------------------------------------------

// NewBlockEvent constructs a BlockEvent with a non-nil metadata map when the
// provided metadata is nil (avoids nil map surprises downstream).
func NewBlockEvent(id, datasource, dsType string, createdAt time.Time, text string, metadata map[string]any) BlockEvent {
	if metadata == nil {
		metadata = make(map[string]any)
	}
	return BlockEvent{
		ID:         id,
		Datasource: datasource,
		DSType:     dsType,
		CreatedAt:  createdAt,
		Text:       text,
		Metadata:   metadata,
	}
}

// WrapBlock produces an InternalEvent for a given BlockEvent.
func WrapBlock(be BlockEvent) InternalEvent {
	return InternalEvent{
		Type:  "block",
		Block: be,
	}
}
