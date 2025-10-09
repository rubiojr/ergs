package api

import (
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/storage"
)

// Gorilla WebSocket upgrader (replaces manual RFC6455 implementation)
var wsUpgrader = websocket.Upgrader{
	ReadBufferSize:  8192,
	WriteBufferSize: 8192,
	CheckOrigin: func(r *http.Request) bool {
		// Allow all origins for now; tighten if needed.
		return true
	},
}

func (s *Server) RegisterRoutes(mux *http.ServeMux) {
	// API routes with method-specific routing
	mux.HandleFunc("GET /api/datasources", s.HandleListDatasources)
	mux.HandleFunc("GET /api/datasources/{name}", s.HandleDatasourceBlocks)
	mux.HandleFunc("GET /api/search", s.HandleSearch)
	mux.HandleFunc("GET /api/firehose", s.HandleFirehose)
	mux.HandleFunc("GET /api/stats", s.HandleStats)
	mux.HandleFunc("GET /health", s.HandleHealth)

	// WebSocket firehose route
	mux.HandleFunc("GET /api/firehose/ws", s.HandleFirehoseWS)
}

// HandleFirehoseWS upgrades the connection to a WebSocket and streams an initial
// firehose snapshot followed by periodic incremental polls (fallback approach
// when an in-process realtime hub is not directly accessible here).
//
// This implementation performs:
//  1. WebSocket handshake (RFC6455)
//  2. Initial snapshot (equivalent to GET /api/firehose)
//  3. Poll every 5s for new blocks (by timestamp) and push them
//  4. Heartbeat frame (JSON) every 30s to keep connection alive
//
// NOTE: When a direct event hub adapter exposing Register/Unregister becomes
// accessible inside the API server, replace the polling loop with push events.
func (s *Server) HandleFirehoseWS(w http.ResponseWriter, r *http.Request) {
	// Upgrade using gorilla/websocket
	conn, err := wsUpgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Printf("firehose ws: upgrade failed: %v", err)
		return
	}
	defer conn.Close()

	// Initial snapshot (equivalent to firehose REST)
	// Honor optional ?since=RFC3339 (high precision) plus standard limit/page overrides.
	searchService := s.storageManager.GetSearchService()
	rawParams, _ := storage.ParseSearchParams(r.URL.Query()) // ignore error; WebSocket path is lenient
	// Firehose semantics: always empty query (all recent blocks)
	rawParams.Query = ""
	if rawParams.Limit == 0 {
		rawParams.Limit = 30
	}
	if rawParams.Page == 0 {
		rawParams.Page = 1
	}
	params := rawParams
	// If since is provided we force page=1 to return only newest after the boundary.
	if params.Since != nil {
		params.Page = 1
	}

	results, err := searchService.Search(params)
	if err != nil {
		_ = conn.WriteJSON(map[string]any{
			"type":  "error",
			"error": "initial_search_failed",
			"info":  err.Error(),
		})
		return
	}

	type wsBlock struct {
		ID            string                 `json:"id"`
		Text          string                 `json:"text"`
		Source        string                 `json:"source"`
		DSType        string                 `json:"ds_type,omitempty"`
		CreatedAt     time.Time              `json:"created_at"`
		Metadata      map[string]interface{} `json:"metadata,omitempty"`
		FormattedHTML string                 `json:"formatted_html,omitempty"`
	}

	var (
		allBlocks []wsBlock
		newest    time.Time
	)
	for dsName, blocks := range results.Results {
		for _, b := range blocks {
			md := b.Metadata()
			if md == nil {
				md = map[string]interface{}{}
			}
			if _, ok := md["datasource"]; !ok {
				md["datasource"] = dsName
			}
			// Apply 'since' boundary if present.
			// Normalize comparison to second precision so a block whose stored created_at
			// is the same second as the client 'since' cursor (but with greater sub‑second
			// precision) is NOT included (prevents duplicate on reconnect).
			if params.Since != nil {
				if !b.CreatedAt().Truncate(time.Second).After(params.Since.Truncate(time.Second)) {
					continue
				}
			}
			var formatted string
			if s.rendererService != nil {
				if htmlStr, _ := s.rendererService.Render(b); htmlStr != "" {
					formatted = htmlStr
				}
			}
			allBlocks = append(allBlocks, wsBlock{
				ID:            b.ID(),
				Text:          b.Text(),
				Source:        b.Source(),
				DSType:        b.Type(),
				CreatedAt:     b.CreatedAt(),
				Metadata:      md,
				FormattedHTML: formatted,
			})
			if b.CreatedAt().After(newest) {
				newest = b.CreatedAt()
			}
		}
	}

	// Determine (preliminarily) whether push mode is expected (hub supports Register)
	type registerableHub interface {
		Register() (uint64, <-chan InternalEvent)
		Unregister(id uint64)
	}
	pushCapable := false
	if _, ok := s.firehoseHub.(registerableHub); ok && s.firehoseHub != nil {
		pushCapable = true
	}
	mode := "poll"
	if pushCapable {
		mode = "push"
	}

	log.Printf("firehose ws: %s init snapshot blocks=%d mode=%s since=%v", r.RemoteAddr, len(allBlocks), mode, func() any {
		if params.Since != nil {
			return params.Since.UTC()
		}
		return nil
	}())

	if err := conn.WriteJSON(map[string]any{
		"type":   "init",
		"count":  len(allBlocks),
		"blocks": allBlocks,
		"mode":   mode,
		"since": func() any {
			if params.Since != nil {
				return params.Since.UTC()
			}
			return nil
		}(),
	}); err != nil {
		log.Printf("firehose ws: init write failed %s: %v", r.RemoteAddr, err)
		return
	}
	// Advance since cursor to newest we have just sent so fallback polling (if used) only fetches later blocks.
	if newest.IsZero() {
		// If no blocks returned and since provided, keep existing since; else set to current time.
		if params.Since == nil {
			now := time.Now().UTC()
			params.Since = &now
		}
	} else {
		params.Since = &newest
	}

	// Attempt to use realtime hub (push) if available (registerableHub interface already declared above)
	var (
		subID   uint64
		eventsC <-chan InternalEvent
	)
	if rh, okAssert := s.firehoseHub.(interface {
		Register() (uint64, <-chan InternalEvent)
		Unregister(id uint64)
	}); okAssert && rh != nil {
		subID, eventsC = rh.Register()
		log.Printf("firehose ws: %s registered listener id=%d (push mode active)", r.RemoteAddr, subID)
		defer func() {
			rh.Unregister(subID)
			log.Printf("firehose ws: %s unregistered listener id=%d", r.RemoteAddr, subID)
		}()
	} else {
		eventsC = nil // fallback polling mode
		log.Printf("firehose ws: %s operating in polling mode (no push hub)", r.RemoteAddr)
	}

	pollTicker := time.NewTicker(5 * time.Second)
	defer pollTicker.Stop()

	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	// Reader to detect close (we ignore incoming messages)
	go func() {
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				_ = conn.SetReadDeadline(time.Now().Add(-1 * time.Hour))
				return
			}
		}
	}()

	for {
		select {
		case evt, open := <-eventsC:
			if !open {
				eventsC = nil
				continue
			}

			be := evt.Block
			var formatted string
			if s.rendererService != nil {
				gb := core.NewGenericBlock(be.ID, be.Text, be.Datasource, be.DSType, be.CreatedAt, be.Metadata)
				if htmlStr, _ := s.rendererService.Render(gb); htmlStr != "" {
					formatted = htmlStr
				}
			}

			blockPayload := map[string]any{
				"id":         be.ID,
				"text":       be.Text,
				"source":     be.Datasource,
				"ds_type":    be.DSType,
				"created_at": be.CreatedAt,
				"metadata":   be.Metadata,
			}
			if formatted != "" {
				blockPayload["formatted_html"] = formatted
			}

			msg := map[string]any{
				"type":  "block",
				"block": blockPayload,
			}
			if err := conn.WriteJSON(msg); err != nil {
				return
			}
			if be.CreatedAt.After(newest) {
				newest = be.CreatedAt
			}
		case <-pollTicker.C:
			if eventsC != nil {
				continue // hub push active; skip polling
			}
			// For polling mode, always use the evolving since boundary (high precision).
			results, err := searchService.Search(params)
			if err != nil {
				_ = conn.WriteJSON(map[string]any{
					"type":  "error",
					"error": "poll_failed",
					"info":  err.Error(),
				})
				continue
			}
			var newBlocks []wsBlock
			for dsName, blocks := range results.Results {
				for _, b := range blocks {
					// Filter using 'since' boundary at second precision (same logic as init snapshot)
					if params.Since != nil {
						if !b.CreatedAt().Truncate(time.Second).After(params.Since.Truncate(time.Second)) {
							continue
						}
					}
					md := b.Metadata()
					if md == nil {
						md = map[string]interface{}{}
					}
					if _, ok := md["datasource"]; !ok {
						md["datasource"] = dsName
					}
					var formatted string
					if s.rendererService != nil {
						if htmlStr, _ := s.rendererService.Render(b); htmlStr != "" {
							formatted = htmlStr
						}
					}
					newBlocks = append(newBlocks, wsBlock{
						ID:            b.ID(),
						Text:          b.Text(),
						Source:        b.Source(),
						DSType:        b.Type(),
						CreatedAt:     b.CreatedAt(),
						Metadata:      md,
						FormattedHTML: formatted,
					})
					if b.CreatedAt().After(newest) {
						newest = b.CreatedAt()
					}
				}
			}
			if len(newBlocks) > 0 {
				// Advance since cursor to newest emitted block
				if newestPoll := newBlocks[0].CreatedAt; newestPoll.After(*params.Since) {
					// Because results are ordered newest-first, first element is newest
					params.Since = &newBlocks[0].CreatedAt
				}
				if err := conn.WriteJSON(map[string]any{
					"type":   "block_batch",
					"count":  len(newBlocks),
					"blocks": newBlocks,
					"since":  params.Since.UTC(),
				}); err != nil {
					log.Printf("firehose ws: %s block_batch write failed: %v", r.RemoteAddr, err)
					return
				}
			}
		case <-heartbeatTicker.C:
			if err := conn.WriteJSON(map[string]any{
				"type": "heartbeat",
				"ts":   time.Now().UTC().Format(time.RFC3339Nano),
			}); err != nil {
				return
			}
		}
	}
}

// (Removed obsolete duplicate manual WebSocket logic)
