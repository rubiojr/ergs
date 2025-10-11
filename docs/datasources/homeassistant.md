# Home Assistant Datasource

The Home Assistant datasource connects to a running Home Assistant instance through its WebSocket API and ingests recent events into Ergs for unified search, analysis, and cross‑referencing with your other personal data (GitHub, RSS, browser history, etc.).

It captures raw Home Assistant events (e.g. `state_changed`, `call_service`, automations firing, scripts running) and stores compact, searchable metadata along with the raw event payload.

---

## Features

- Authenticated WebSocket connection (long‑lived access token)
- Optional filtering by specific event types
- Compact structured metadata for querying (`event_type`, `entity_id`, `domain`, `service`, etc.)
- Raw `data` preserved as JSON (searchable text, truncated in block text but fully stored in metadata)
- Deterministic block IDs (uses Home Assistant `context.id` when available)
- Custom renderer for concise visual display with expandable payload

---

## Configuration

Add a section to your `config.toml`:

```toml
[datasources.home]
type = 'homeassistant'
interval = '30s'  # Fetch every 30 seconds (tune to your needs)

[datasources.home.config]
url = 'ws://homeassistant.local:8123/api/websocket'
token = 'YOUR_LONG_LIVED_ACCESS_TOKEN'
max_events = 200
event_types = ['state_changed', 'call_service']  # Optional; omit for all
entity_ids = ['sensor.living_room_temp', 'sensor.cpu_load']  # Optional: only keep events for these entity_ids
idle_timeout = '5m'  # Optional: end fetch early if no events arrive for this period (default 5m)
```

### Configuration Options

| Field          | Type     | Default                                             | Description |
|----------------|----------|-----------------------------------------------------|-------------|
| `url`          | string   | `ws://homeassistant.local:8123/api/websocket`       | WebSocket endpoint (must include scheme and host; `wss://` for TLS) |
| `token`        | string   | (required)                                          | Long‑lived access token from Home Assistant user profile |
| `event_types`  | array    | `[]`                                                | If empty: subscribe to all event types; else one subscription per item |
| `entity_ids`   | array    | `[]`                                                | If non-empty: only keep events whose `entity_id` matches one of these |
| `max_events`   | int      | `100` (capped at 1000)                              | Maximum events collected per fetch cycle |
| `idle_timeout` | duration | `5m`                                                | End a fetch early if no events are received for this period (prevents silent hangs) |

---

## Obtaining a Long‑Lived Access Token

1. Open your Home Assistant UI
2. Click your user profile (bottom-left)
3. Scroll to "Long-Lived Access Tokens"
4. Create a new token (name it something like "Ergs Ingestion")
5. Copy the token (you will not be able to view it again)
6. Paste it into `config.toml` under `[datasources.<name>.config].token`

Tokens are tied to your user account and carry that user's permissions.

---

## Data Model

Each Home Assistant event becomes a block with the following metadata fields:

| Field              | Type   | Description |
|--------------------|--------|-------------|
| `event_type`       | string | Event type (e.g., `state_changed`, `call_service`) |
| `entity_id`        | string | Entity affected (if present in event data) |
| `domain`           | string | Derived from `entity_id` or `data.domain` |
| `service`          | string | Service name for `call_service` events |
| `origin`           | string | Event origin reported by HA |
| `time_fired`       | string | Original ISO timestamp from HA |
| `context_id`       | string | Home Assistant event context ID |
| `context_user_id`  | string | User ID (if event originated from a user) |
| `data_json`        | string | Raw `event.data` JSON |
| (core fields)      | —      | `id`, `text`, `created_at`, `datasource`, `type` |

The block `Text()` is a compact searchable string like:

```
event_type=state_changed entity_id=light.living_room domain=light data={"new_state":"on"...}
```

Raw JSON is always stored in `data_json` even if truncated in text.

---

## Search Examples

Find all service calls turning on lights:
```
event_type:call_service domain:light service:turn_on
```

All state changes for a specific entity:
```
event_type:state_changed entity_id:light.kitchen_ceiling
```

Anything involving automations:
```
domain:automation OR event_type:automation_triggered
```

Events involving climate control:
```
domain:climate
```

User‑initiated actions (if `context_user_id` present):
```
context_user_id:* service:turn_on
```

Search for motion:
```
data_json:motion OR entity_id:binary_sensor.motion*
```

Combine with time filtering using CLI options (`--since`, `--limit`, etc.).

---

## Recommended Event Types

If you choose to filter (`event_types`), common useful values include:
- `state_changed`
- `call_service`
- `automation_triggered`
- `script_started`
- `scene_reloaded`
- `timer.finished`
- `mobile_app_notification_action`

Leave `event_types` empty to capture everything (useful for discovery; then narrow).

---

## Scheduling & Interval Guidance

| Use Case | Recommended `interval` | Notes |
|----------|------------------------|-------|
| Low traffic / experimentation | `1m`–`5m` | Fewer open/close cycles, fewer events lost |
| Moderate household activity | `30s`–`1m` | Good balance for timely ingestion |
| High‑frequency / sensors dense | `15s`–`30s` | Consider reducing `max_events` to keep cycles fast |

Because each fetch is bounded by `max_events`, extremely busy environments might benefit from shorter intervals to avoid missing bursts.

---

## Renderer

The custom HTML renderer:
- Shows event type, entity, domain, service
- Relative time since firing
- Context and user (when available)
- Expandable raw JSON payload
- Applies the shared UI palette (no custom colors outside existing CSS tokens)

---

## Troubleshooting

| Issue | Cause | Resolution |
|-------|-------|-----------|
| Auth fails (`auth_invalid`) | Wrong/missing token | Regenerate token & update config |
| No events ingested | Empty filter / mismatch | Remove `event_types` to test; verify HA activity |
| Fetch seems stuck after "connecting" | Bad URL (missing //) or network stall | Ensure URL is like `ws://host:8123/api/websocket` (two slashes) |
| Missing entity IDs | Some event types lack `entity_id` | Check `data_json` for other identifiers |
| Duplicate IDs | Rare (context reuse) | Falls back to synthesized ID from time + type |
| TLS errors | Using `wss://` behind reverse proxy | Verify proxy supports WebSockets and correct path |

Run verbose logs:
```
ERG_LOG=debug ergs fetch --datasource home
```
(If the project later adds structured logging, adapt accordingly.)

---

## Security Notes

- The long‑lived access token grants API access equal to your Home Assistant user.
- Treat the `config.toml` as sensitive.
- Prefer local network usage; if using remote access ensure TLS (`wss://`) and a trusted reverse proxy.

---

## Example Minimal Configuration

```toml
[datasources.home_ha]
type = 'homeassistant'
[datasources.home_ha.config]
token = 'REPLACE_ME'
```

This uses defaults:
- URL: `ws://homeassistant.local:8123/api/websocket`
- `max_events = 100`
- All event types

---

## Future Enhancements (Potential)

(Informational only; not implemented unless requested)
- Continuous streaming mode (long‑lived connection)
- Entity state snapshot introspection
- Deriving humanized change summaries (old → new)
- Domain‑specific faceting (lights vs climate vs security)

---

## See Also

- Official WebSocket API docs: https://developers.home-assistant.io/docs/api/websocket/
- Datasource development guide: `docs/datasource.md`
- Search usage: `ergs search --help`

---

Happy automating! Combine HA events with your digital activity timeline for richer context (e.g., “What code was I committing when the hallway motion started triggering at midnight?”).