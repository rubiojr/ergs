// Package homeassistant implements a datasource that connects to a Home Assistant
// instance via its WebSocket API and captures recent events.
//
// References:
//   - https://developers.home-assistant.io/docs/api/websocket/
//
// Design Notes:
//
//   - FetchBlocks performs a bounded capture of events (MaxEvents) and then returns.
//     This matches the pull-oriented contract used by other datasources. If you need
//     continuous ingestion, schedule the datasource at a short interval.
//   - Authentication uses a long‑lived access token (required).
//   - Subscriptions: If EventTypes is empty we subscribe to all events. Otherwise we
//     issue a separate subscribe command per event type.
//   - Each received event (type == "event") is converted into a Block. The block ID
//     prefers the Home Assistant context.id (if present) for determinism. If absent,
//     we derive one from eventType + time_fired.
//   - Metadata is intentionally compact but includes raw event data (stringified JSON)
//     for advanced searches.
//   - Idle timeout: If no events are received for IdleTimeout the fetch ends early.
//
// Configuration Example (config.toml):
//
//	[datasources.ha]
//	type = 'homeassistant'
//	interval = '30s'
//
//	[datasources.ha.config]
//	url = 'ws://homeassistant.local:8123/api/websocket'
//	token = 'YOUR_LONG_LIVED_ACCESS_TOKEN'
//	max_events = 200
//	event_types = ['state_changed', 'call_service']
//	idle_timeout = '5m'
//
// Schema Fields:
//
//	event_type       TEXT
//	entity_id        TEXT
//	domain           TEXT
//	service          TEXT
//	origin           TEXT
//	time_fired       TEXT (original timestamp string)
//	context_id       TEXT
//	context_user_id  TEXT
//	data_json        TEXT (raw event.data JSON)
//
// Search Examples:
//
//	event_type:state_changed domain:light
//	service:turn_on entity_id:light.living_room
//	event_type:call_service domain:climate
//
// Renderer:
//
//	A custom renderer should be added in renderer/ (not in this file) to keep parity
//	with other datasources. This file focuses on ingestion logic.
package homeassistant

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/url"
	"strings"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/log"
)

func init() {
	prototype := &Datasource{}
	core.RegisterDatasourcePrototype("homeassistant", prototype)
}

// Config holds user‑defined settings for the Home Assistant datasource.
type Config struct {
	URL         string        `toml:"url"`          // WebSocket endpoint (ws[s]://.../api/websocket)
	Token       string        `toml:"token"`        // Long-lived access token (required)
	EventTypes  []string      `toml:"event_types"`  // Optional filter list; empty => all events
	EntityIDs   []string      `toml:"entity_ids"`   // Optional: only keep events whose entity_id matches any of these; empty => no entity filter
	MaxEvents   int           `toml:"max_events"`   // Maximum events to capture per fetch (default 100, hard cap 1000)
	IdleTimeout time.Duration `toml:"idle_timeout"` // Max idle period with no events before returning (default 5m)
}

// Validate sets defaults and verifies required fields.
func (c *Config) Validate() error {
	if c.URL == "" {
		c.URL = "ws://homeassistant.local:8123/api/websocket"
	}
	if c.Token == "" {
		return fmt.Errorf("homeassistant: token is required (create a long-lived access token in your user profile)")
	}
	if c.MaxEvents <= 0 {
		c.MaxEvents = 100
	}
	if c.MaxEvents > 1000 {
		c.MaxEvents = 1000
	}
	if c.IdleTimeout <= 0 {
		c.IdleTimeout = 5 * time.Minute
	}
	// Validate URL structure (A)
	u, err := url.Parse(c.URL)
	if err != nil || (u.Scheme != "ws" && u.Scheme != "wss") || u.Host == "" {
		return fmt.Errorf("homeassistant: invalid websocket URL %q", c.URL)
	}
	// Normalize entity IDs to lower-case (Home Assistant entity_ids are case-insensitive but usually lower)
	for i, e := range c.EntityIDs {
		c.EntityIDs[i] = strings.ToLower(strings.TrimSpace(e))
	}
	return nil
}

// Datasource implements core.Datasource for Home Assistant.
type Datasource struct {
	config       *Config
	instanceName string
}

// NewDatasource constructs a new Home Assistant datasource instance.
func NewDatasource(instanceName string, config interface{}) (core.Datasource, error) {
	var haConfig *Config
	if config == nil {
		// Registry creates datasource with nil config first; defer validation until SetConfig.
		// This avoids failing creation due to missing token before user config is applied.
		haConfig = &Config{}
	} else {
		var ok bool
		haConfig, ok = config.(*Config)
		if !ok {
			return nil, fmt.Errorf("homeassistant: invalid config type")
		}
		// Only validate when an explicit config struct is provided.
		if err := haConfig.Validate(); err != nil {
			return nil, err
		}
	}

	return &Datasource{
		config:       haConfig,
		instanceName: instanceName,
	}, nil
}

// Type returns the datasource type identifier.
func (d *Datasource) Type() string { return "homeassistant" }

// Name returns the instance name (user-defined in config).
func (d *Datasource) Name() string { return d.instanceName }

// Schema defines the DB schema for this datasource.
func (d *Datasource) Schema() map[string]any {
	return map[string]any{
		"event_type":      "TEXT",
		"entity_id":       "TEXT",
		"domain":          "TEXT",
		"service":         "TEXT",
		"origin":          "TEXT",
		"time_fired":      "TEXT",
		"context_id":      "TEXT",
		"context_user_id": "TEXT",
		"data_json":       "TEXT",
	}
}

// BlockPrototype returns a prototype block for reconstruction.
func (d *Datasource) BlockPrototype() core.Block {
	return &EventBlock{}
}

// ConfigType returns a pointer to an empty Config for decoding.
func (d *Datasource) ConfigType() interface{} { return &Config{} }

// SetConfig updates the datasource configuration at runtime.
func (d *Datasource) SetConfig(config interface{}) error {
	cfg, ok := config.(*Config)
	if !ok {
		return fmt.Errorf("homeassistant: invalid config type")
	}
	if err := cfg.Validate(); err != nil {
		return err
	}
	d.config = cfg
	return nil
}

// GetConfig returns the current configuration.
func (d *Datasource) GetConfig() interface{} { return d.config }

// Close performs cleanup (none needed for now).
func (d *Datasource) Close() error { return nil }

// Factory creates a new datasource instance (core requirement).
func (d *Datasource) Factory(instanceName string, config interface{}) (core.Datasource, error) {
	return NewDatasource(instanceName, config)
}

// haInboundMessage represents generic messages from HA.
type haInboundMessage struct {
	Type  string           `json:"type"`
	ID    int              `json:"id,omitempty"`
	Event *haEventEnvelope `json:"event,omitempty"`
}

// haEventEnvelope wraps an event payload.
type haEventEnvelope struct {
	EventType string                 `json:"event_type"`
	Data      map[string]interface{} `json:"data"`
	Origin    string                 `json:"origin"`
	TimeFired string                 `json:"time_fired"`
	Context   haContext              `json:"context"`
}

// haContext contains event context info.
type haContext struct {
	ID     string      `json:"id"`
	UserID interface{} `json:"user_id"`   // nullable
	Parent interface{} `json:"parent_id"` // nullable
}

// authRequiredMsg and authOkMsg types are inferred by Type fields; no dedicated structs needed.

// FetchBlocks connects to HA, authenticates, subscribes, and receives up to MaxEvents events.
func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
	l := log.ForService("homeassistant:" + d.instanceName)
	l.Debugf("connecting to %s", d.config.URL)

	u, err := url.Parse(d.config.URL)
	if err != nil {
		return fmt.Errorf("homeassistant: invalid URL: %w", err)
	}

	// Overall dial timeout (B)
	dialCtx, cancel := context.WithTimeout(ctx, 20*time.Second)
	defer cancel()

	dialer := websocket.Dialer{
		Proxy:            websocket.DefaultDialer.Proxy,
		HandshakeTimeout: 15 * time.Second,
	}

	conn, _, err := dialer.DialContext(dialCtx, u.String(), nil)
	if err != nil {
		return fmt.Errorf("homeassistant: websocket dial failed: %w", err)
	}
	defer func() { _ = conn.Close() }()

	// Step 1: Expect auth_required
	var msg haInboundMessage

	// Set up a channel to read auth_required with context support
	type authResult struct {
		msg haInboundMessage
		err error
	}
	authCh := make(chan authResult, 1)

	go func() {
		if err := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
			l.Warnf("set read deadline: %v", err)
		}
		var m haInboundMessage
		err := conn.ReadJSON(&m)
		authCh <- authResult{msg: m, err: err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-authCh:
		if result.err != nil {
			return fmt.Errorf("homeassistant: reading auth_required: %w", result.err)
		}
		msg = result.msg
		if msg.Type != "auth_required" {
			return fmt.Errorf("homeassistant: expected auth_required, got %s", msg.Type)
		}
	}

	// Step 2: Send auth
	authPayload := map[string]string{
		"type":         "auth",
		"access_token": d.config.Token,
	}
	if err := conn.WriteJSON(authPayload); err != nil {
		return fmt.Errorf("homeassistant: send auth failed: %w", err)
	}

	// Step 3: Expect auth_ok / auth_invalid
	authRespCh := make(chan authResult, 1)

	go func() {
		if err := conn.SetReadDeadline(time.Now().Add(15 * time.Second)); err != nil {
			l.Warnf("set read deadline: %v", err)
		}
		var m haInboundMessage
		err := conn.ReadJSON(&m)
		authRespCh <- authResult{msg: m, err: err}
	}()

	select {
	case <-ctx.Done():
		return ctx.Err()
	case result := <-authRespCh:
		if result.err != nil {
			return fmt.Errorf("homeassistant: reading auth response: %w", result.err)
		}
		switch result.msg.Type {
		case "auth_ok":
			// proceed
		case "auth_invalid":
			return fmt.Errorf("homeassistant: authentication failed (auth_invalid)")
		default:
			return fmt.Errorf("homeassistant: unexpected auth phase message: %s", result.msg.Type)
		}
	}

	// Step 4: Subscribe
	subID := 1
	if len(d.config.EventTypes) == 0 {
		// Subscribe to all events
		subAll := map[string]interface{}{
			"id":   subID,
			"type": "subscribe_events",
		}
		if err := conn.WriteJSON(subAll); err != nil {
			return fmt.Errorf("homeassistant: subscribe all events failed: %w", err)
		}
		l.Debugf("subscribed to all events")
	} else {
		for _, et := range d.config.EventTypes {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			sub := map[string]interface{}{
				"id":         subID,
				"type":       "subscribe_events",
				"event_type": et,
			}
			if err := conn.WriteJSON(sub); err != nil {
				return fmt.Errorf("homeassistant: subscribe event %q failed: %w", et, err)
			}
			subID++
			time.Sleep(25 * time.Millisecond) // gentle pacing
		}
		l.Debugf("subscribed to %d event types", len(d.config.EventTypes))
	}

	// Step 5: Capture events
	received := 0
	start := time.Now()
	lastEvent := start
	idleTimeout := d.config.IdleTimeout

	// Create channels for communication
	type readResult struct {
		msg haInboundMessage
		err error
	}
	readCh := make(chan readResult)

	// Start goroutine for reading messages
	go func() {
		for {
			// Use shorter deadline to allow more frequent context checks
			if err := conn.SetReadDeadline(time.Now().Add(5 * time.Second)); err != nil {
				l.Warnf("set read deadline: %v", err)
			}

			var im haInboundMessage
			err := conn.ReadJSON(&im)

			select {
			case readCh <- readResult{msg: im, err: err}:
			case <-ctx.Done():
				return
			}

			// Check if we should stop reading
			if err != nil {
				return
			}
		}
	}()

	for received < d.config.MaxEvents {
		select {
		case <-ctx.Done():
			l.Debugf("context cancelled, stopping")
			return ctx.Err()

		case result := <-readCh:
			if result.err != nil {
				// Check if error is due to context cancellation
				select {
				case <-ctx.Done():
					return ctx.Err()
				default:
					// Idle timeout (C): On read timeout, check idle duration
					if netErr, ok := result.err.(net.Error); ok && netErr.Timeout() {
						if time.Since(lastEvent) >= idleTimeout {
							l.Debugf("idle timeout %v reached (captured %d events)", idleTimeout, received)
							l.Debugf("captured %d events", received)
							return nil
						}
						continue
					}
					return fmt.Errorf("homeassistant: read message: %w", result.err)
				}
			}

			// Ignore non-event messages
			if result.msg.Type != "event" || result.msg.Event == nil {
				continue
			}

			block, err := d.convertEventToBlock(result.msg.Event)
			if err != nil {
				l.Warnf("convert event failed: %v", err)
				continue
			}

			// Entity filtering (only for blocks with an entity_id when configured)
			if len(d.config.EntityIDs) > 0 {
				if eb, ok := block.(*EventBlock); ok {
					if eb.entityID == "" || !d.entityAllowed(eb.entityID) {
						continue
					}
				}
			}

			select {
			case <-ctx.Done():
				return ctx.Err()
			case blockCh <- block:
				received++
				lastEvent = time.Now()
			}
		}
	}

	l.Debugf("captured %d events", received)
	return nil
}

// convertEventToBlock transforms an HA event envelope into a Block.
func (d *Datasource) convertEventToBlock(ev *haEventEnvelope) (core.Block, error) {
	eventType := ev.EventType
	rawTime := ev.TimeFired
	createdAt := parseHATime(rawTime)

	// Extract commonly useful fields
	var entityID, domain, service string
	if ev.Data != nil {
		if v, ok := ev.Data["entity_id"].(string); ok {
			entityID = v
		}
		if v, ok := ev.Data["service"].(string); ok {
			service = v
		}
		// domain often derivable from entity_id "domain.object"
		if entityID != "" && strings.Contains(entityID, ".") {
			domain = strings.SplitN(entityID, ".", 2)[0]
		} else if v, ok := ev.Data["domain"].(string); ok {
			domain = v
		}
	}

	contextID := ev.Context.ID
	contextUserID := ""
	if uid, ok := ev.Context.UserID.(string); ok {
		contextUserID = uid
	}

	// Raw data JSON
	dataJSON := ""
	if ev.Data != nil {
		if b, err := json.Marshal(ev.Data); err == nil {
			dataJSON = string(b)
		}
	}

	// Build ID (prefer context ID, else eventType + time)
	id := contextID
	if id == "" {
		safeTime := strings.ReplaceAll(rawTime, ":", "_")
		safeTime = strings.ReplaceAll(safeTime, ".", "_")
		id = fmt.Sprintf("%s-%s", eventType, safeTime)
	}

	// Searchable text
	textParts := []string{
		"event_type=" + eventType,
	}
	if entityID != "" {
		textParts = append(textParts, "entity_id="+entityID)
	}
	if domain != "" {
		textParts = append(textParts, "domain="+domain)
	}
	if service != "" {
		textParts = append(textParts, "service="+service)
	}
	if dataJSON != "" {
		// Keep data maybe truncated to avoid runaway size
		short := dataJSON
		if len(short) > 400 {
			short = short[:400] + "...(truncated)"
		}
		textParts = append(textParts, "data="+short)
	}

	text := strings.Join(textParts, " ")

	metadata := map[string]interface{}{
		"event_type":      eventType,
		"entity_id":       entityID,
		"domain":          domain,
		"service":         service,
		"origin":          ev.Origin,
		"time_fired":      rawTime,
		"context_id":      contextID,
		"context_user_id": contextUserID,
		"data_json":       dataJSON,
	}

	return NewEventBlock(
		id,
		text,
		d.instanceName,
		createdAt,
		metadata,
		eventType,
		entityID,
		domain,
		service,
		ev.Origin,
		rawTime,
		contextID,
		contextUserID,
		dataJSON,
	), nil
}

func (d *Datasource) entityAllowed(entityID string) bool {
	if len(d.config.EntityIDs) == 0 {
		return true
	}
	e := strings.ToLower(entityID)
	for _, allowed := range d.config.EntityIDs {
		if e == allowed {
			return true
		}
	}
	return false
}

// parseHATime attempts to parse Home Assistant's time_fired value.
func parseHATime(ts string) time.Time {
	if ts == "" {
		return time.Now().UTC()
	}
	// Common formats include microseconds and timezone offset.
	formats := []string{
		time.RFC3339Nano,
		time.RFC3339,
		"2006-01-02T15:04:05.999999-07:00",
		"2006-01-02T15:04:05-07:00",
	}
	for _, f := range formats {
		if t, err := time.Parse(f, ts); err == nil {
			return t.UTC()
		}
	}
	return time.Now().UTC()
}

/////////////////////////////////////////////////////
// Block Implementation (EventBlock)
/////////////////////////////////////////////////////

// EventBlock represents a single Home Assistant event.
type EventBlock struct {
	id            string
	text          string
	createdAt     time.Time
	source        string
	metadata      map[string]interface{}
	eventType     string
	entityID      string
	domain        string
	service       string
	origin        string
	timeFiredRaw  string
	contextID     string
	contextUserID string
	dataJSON      string
}

// NewEventBlock constructs a new EventBlock.
func NewEventBlock(
	id string,
	text string,
	source string,
	createdAt time.Time,
	metadata map[string]interface{},
	eventType, entityID, domain, service, origin, timeFiredRaw, contextID, contextUserID, dataJSON string,
) *EventBlock {
	return &EventBlock{
		id:            id,
		text:          text,
		createdAt:     createdAt,
		source:        source,
		metadata:      metadata,
		eventType:     eventType,
		entityID:      entityID,
		domain:        domain,
		service:       service,
		origin:        origin,
		timeFiredRaw:  timeFiredRaw,
		contextID:     contextID,
		contextUserID: contextUserID,
		dataJSON:      dataJSON,
	}
}

// Interface compliance

func (e *EventBlock) ID() string                       { return e.id }
func (e *EventBlock) Text() string                     { return e.text }
func (e *EventBlock) CreatedAt() time.Time             { return e.createdAt }
func (e *EventBlock) Source() string                   { return e.source }
func (e *EventBlock) Metadata() map[string]interface{} { return e.metadata }
func (e *EventBlock) Type() string                     { return "homeassistant" }

// Summary returns a concise one‑liner.
func (e *EventBlock) Summary() string {
	target := e.entityID
	if target == "" {
		target = e.domain
	}
	if target != "" {
		return fmt.Sprintf("🏠 %s %s", e.eventType, target)
	}
	return fmt.Sprintf("🏠 %s", e.eventType)
}

// PrettyText returns a nicely formatted multi-line string.
func (e *EventBlock) PrettyText() string {
	var b strings.Builder
	b.WriteString("🏠 Home Assistant Event\n")
	b.WriteString(fmt.Sprintf("  ID: %s\n", e.id))
	b.WriteString(fmt.Sprintf("  Time: %s\n", e.createdAt.Format("2006-01-02 15:04:05")))
	b.WriteString(fmt.Sprintf("  Type: %s\n", e.eventType))
	if e.entityID != "" {
		b.WriteString(fmt.Sprintf("  Entity: %s\n", e.entityID))
	}
	if e.domain != "" {
		b.WriteString(fmt.Sprintf("  Domain: %s\n", e.domain))
	}
	if e.service != "" {
		b.WriteString(fmt.Sprintf("  Service: %s\n", e.service))
	}
	if e.origin != "" {
		b.WriteString(fmt.Sprintf("  Origin: %s\n", e.origin))
	}
	if e.contextID != "" {
		b.WriteString(fmt.Sprintf("  Context ID: %s\n", e.contextID))
	}
	if e.contextUserID != "" {
		b.WriteString(fmt.Sprintf("  User ID: %s\n", e.contextUserID))
	}

	// Include truncated data preview
	if e.dataJSON != "" {
		preview := e.dataJSON
		if len(preview) > 300 {
			preview = preview[:300] + "...(truncated)"
		}
		b.WriteString("  Data: " + preview + "\n")
	}

	metadataInfo := core.FormatMetadata(e.metadata)
	if metadataInfo != "" {
		b.WriteString(metadataInfo)
	}

	return b.String()
}

// Factory reconstructs an EventBlock from a GenericBlock + source.
func (e *EventBlock) Factory(generic *core.GenericBlock, source string) core.Block {
	md := generic.Metadata()
	getStr := func(k string) string {
		if v, ok := md[k]; ok {
			if s, ok := v.(string); ok {
				return s
			}
		}
		return ""
	}

	return &EventBlock{
		id:            generic.ID(),
		text:          generic.Text(),
		createdAt:     generic.CreatedAt(),
		source:        source,
		metadata:      md,
		eventType:     getStr("event_type"),
		entityID:      getStr("entity_id"),
		domain:        getStr("domain"),
		service:       getStr("service"),
		origin:        getStr("origin"),
		timeFiredRaw:  getStr("time_fired"),
		contextID:     getStr("context_id"),
		contextUserID: getStr("context_user_id"),
		dataJSON:      getStr("data_json"),
	}
}
