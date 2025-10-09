# Event Bridge & Real-Time Firehose Streaming

This document describes the internal **Event Bridge** mechanism that enables real-time streaming of newly ingested blocks from the warehouse process (`ergs serve`) to the web/API process (`ergs web`), and how the WebSocket firehose endpoint (`/api/firehose/ws`) exposes those events to clients.

---

## Overview

Ergs separates responsibilities across two long-running processes:

| Process | Responsibility |
|---------|----------------|
| `ergs serve` | Fetching/ingesting data from datasources, storing blocks in per-datasource SQLite databases, optimization, migrations. |
| `ergs web`   | Serving REST API endpoints, Web UI, full-text search queries, and real-time streaming over WebSockets. |

Because these processes are **not** co-resident in the same address space, we can't simply share in-memory channels to push new blocks directly to the web server. The **Event Bridge** solves this by using a **Unix Domain Socket (UDS)** transport to push serialized block events from the warehouse to any number of subscriber processes (currently the web/API process).

---

## Key Components

1. **Event Bridge (warehouse side)**  
   - Located in the warehouse process; starts when `event_socket_path` is configured in the global config.
   - Publishes newline-delimited JSON (NDJSON) events for each successfully stored block.
   - Emits periodic heartbeat events to help consumers detect stale connections.
   - One-way only: warehouse → consumers (clients send nothing / ignored if they do).

2. **Bridge Consumer (web side)**  
   - Connects to the Unix socket, auto-reconnects with exponential backoff.
   - Parses incoming NDJSON events, converts them to internal events, and forwards them to the in-process **Firehose Hub**.

3. **Firehose Hub (web side)**  
   - A lightweight publish/subscribe dispatcher.
   - Each WebSocket session registers a buffered channel to receive block events.
   - Drops events for a session if its buffer is full (non-blocking design).

4. **WebSocket Firehose Endpoint**: `GET /api/firehose/ws`  
   - Sends an initial snapshot (`init`) containing the most recent blocks.
   - Streams subsequent `block` events pushed through the hub.
   - Emits `heartbeat` messages periodically.
   - Falls back to periodic polling (sending `block_batch`) only if the real-time bridge is disabled.

---

## Configuration

Add `event_socket_path` to your `config.toml` (used by both `ergs serve` and `ergs web`):

```toml
# Enables realtime block streaming from warehouse to web.
event_socket_path = "/run/ergs/bridge.sock"
```

If omitted or empty:
- The warehouse does not start the bridge.
- The web process cannot subscribe; WebSocket endpoint uses a fallback polling mode.

### Permissions

Ensure the directory containing the socket is writable by the warehouse process and readable by the web process.  
Recommended (systemd):  
- Use `RuntimeDirectory=ergs` to get `/run/ergs/` automatically.
- Adjust group ownership or ACLs if running under separate users.

---

## Event Format (NDJSON)

Each line is a JSON object:

### Block Event
```json
{
  "type": "block",
  "id": "unique-block-id",
  "datasource": "datasource_instance_name",
  "created_at": "2025-01-01T12:00:00Z",
  "text": "The searchable text",
  "metadata": {
    "key": "value",
    "datasource_specific": true
  }
}
```

### Heartbeat Event
```json
{
  "type": "heartbeat",
  "ts": "2025-01-01T12:05:30.123456789Z"
}
```

These events are **best effort**:
- No ACK / replay semantics.
- Consumers must perform an initial REST backfill to recover missed data after downtime.

---

## WebSocket Message Types

| Type         | Source              | Description |
|--------------|---------------------|-------------|
| `init`       | Web/API on connect  | Initial snapshot (recent blocks) |
| `block`      | Event Bridge → Hub → WS | Single new block (push mode) |
| `block_batch`| Fallback polling    | Batched new blocks (when real-time disabled) |
| `heartbeat`  | Web/API             | Keep-alive for client |
| `error`      | Web/API             | Non-fatal issue during streaming |

### Example `init`
```json
{
  "type": "init",
  "count": 30,
  "blocks": [ { "...": "..." } ]
}
```

### Example Real-Time Block
```json
{
  "type": "block",
  "block": {
    "id": "abc123",
    "text": "Something happened",
    "source": "github",
    "created_at": "2025-01-01T12:00:05Z",
    "metadata": { "datasource": "github" }
  }
}
```

---

## Lifecycle & Flow

1. Warehouse ingests / fetches a block.
2. Block is persisted to the appropriate datasource DB.
3. Warehouse calls `eventBridge.publishBlock(...)` → serializes + broadcasts NDJSON line.
4. Web Bridge Consumer receives, parses, and pushes into Firehose Hub.
5. Each active WebSocket session receives the block (if buffer not full).
6. Client updates UI/state; optional deduplication via `(source, id)` key.

---

## Failure Modes & Recovery

| Scenario | Effect | Mitigation |
|----------|--------|------------|
| Socket missing at web startup | No real-time | Web relies on polling; ensure warehouse started first |
| Web process restarts | Momentary gap | On reconnect, initial snapshot covers recent history |
| Warehouse restart | Short disconnect | Bridge recreated; consumer reconnects with backoff |
| Slow WebSocket client | Per-session drops | Client should reduce processing latency / increase buffer client-side |
| Large metadata explosion | Bigger events | Consider pruning metadata before broadcasting (future improvement) |

---

## Design Choices

| Aspect | Decision | Rationale |
|--------|----------|-----------|
| Transport | Unix Domain Socket | Low latency, local-only security boundary |
| Format | NDJSON | Stream-friendly, easy to debug |
| Backpressure | Drop per slow listener | Never block ingestion or other listeners |
| Durability | None (ephemeral) | Keeps system lightweight; REST remains source of truth |
| AuthN | None (for now) | Local-only; can be extended later |
| Error Propagation | Silent drops logged | Avoid cascading ingestion failures |

---

## When To Use a Broker Instead

Consider introducing Redis Streams, NATS, or Kafka if you need:
- Cross-host scaling
- Guaranteed ordering per partition
- Replay (offset-based)
- Multi-consumer groups with persistence

The current bridge is optimized for **single-host, low-latency developer and personal infrastructure** scenarios.

---

## Implementing a Client

### JavaScript (Browser)
```js
const ws = new WebSocket("ws://localhost:8080/api/firehose/ws");
const seen = new Set();

ws.onmessage = ev => {
  const msg = JSON.parse(ev.data);
  switch (msg.type) {
    case "init":
      msg.blocks.forEach(handleBlock);
      break;
    case "block":
      handleBlock(msg.block);
      break;
    case "block_batch":
      msg.blocks.forEach(handleBlock);
      break;
    case "heartbeat":
      // update UI / liveness timestamp
      break;
    case "error":
      console.warn("Firehose error:", msg.error, msg.info);
      break;
  }
};

function handleBlock(b) {
  const key = `${b.source}:${b.id}`;
  if (seen.has(key)) return;
  seen.add(key);
  // Render or queue block
  console.log("New block:", b);
}
```

### Reconnect Strategy
- Exponential backoff (e.g., 1s → 2s → 4s → cap at 30s).
- On reconnect, treat the new `init` snapshot as authoritative for the recent window.

---

## Operational Tips

1. **Systemd Unit (warehouse)**  
   - Add `RuntimeDirectory=ergs`  
   - Ensure `ExecStart` includes config with `event_socket_path=/run/ergs/bridge.sock`

2. **Permissions**  
   - If processes run as different users, set group ownership on `/run/ergs` and apply `chmod 0770`.

3. **Monitoring**  
   - Tail logs for phrases like `Event bridge started` and `bridge consumer: connected`.
   - Optionally add metrics counters (future enhancement).

4. **Security Hardening** (Future)  
   - Add optional shared secret handshake over the socket.
   - Restrict WebSocket origins.

---

## Extensibility

You can extend the bridge for:
- Additional event types (`datasource_update`, `schema_change`).
- Structured versioning (`protocol_version` field).
- Backfill request handshake (client sends last seen timestamp—requires protocol change).

---

## FAQ

**Q: Will I lose data if the web process is down?**  
No block data is lost: the REST API reads from SQLite; only ephemeral *events* are missed.

**Q: Can I run multiple web processes?**  
Yes, each can connect to the same socket. All will receive identical event streams (load/dup filtering becomes your responsibility).

**Q: What if I need strict ordering?**  
Introduce a monotonically increasing sequence (e.g., store a synthetic seq in an events table) or move to a broker.

---

## Summary

The Event Bridge offers:
- Simple, low-latency push pipeline.
- Minimal resource overhead.
- Clean separation between ingestion and delivery.
- Safe degradation to polling when disabled.

Use it when you want real-time UI updates or streaming integrations without the operational weight of an external message bus.

---

Happy streaming! If you need broker-grade durability or multi-cluster distribution later, this architecture provides a clean point to swap out the transport without rewriting ingestion logic.