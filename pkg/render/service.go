package render

import (
	"html"
	"html/template"

	"github.com/rubiojr/ergs/pkg/core"
)

/*
Service (Rendering Pipeline)

Goals:
  * Single import path for all rendering responsibilities
  * Reusable from:
      - cmd/web (server-side templated HTML pages)
      - pkg/api (WebSocket enrichment / future REST pre-rendering)
      - tests / tooling
  * Zero hidden global state (registry passed in / owned by caller)
  * Graceful fallback when no registry is provided
  * Stable, minimal surface: Render(block) -> (html, links)

Security / Trust Model:
  * BlockRenderer implementations are responsible for returning trusted HTML
    (already sanitized / escaped as appropriate).
  * The Service does NOT re-sanitize renderer output; it treats it as final.
  * When no registry is configured, we fall back to an escaped <pre> variant
    of the block's raw text to avoid accidental raw HTML injection.

Extensibility:
  * Future enhancements (caching, instrumentation, sanitizer hook) can wrap
    or extend Service without breaking current callers.
*/

// Service renders blocks to HTML and derives lightweight presentation metadata
// (currently just outbound links). It is concurrency-safe for read-only usage
// because the underlying registry is internally synchronized.
type Service struct {
	registry *RendererRegistry
}

// NewService creates a new Service with the provided registry. The registry
// may be nil; in that case a defensive plain-text fallback is used.
func NewService(reg *RendererRegistry) *Service {
	return &Service{registry: reg}
}

// New is an alias for NewService for brevity in call sites.
func New(reg *RendererRegistry) *Service {
	return NewService(reg)
}

// Render converts a core.Block to:
//   - htmlOut: trusted HTML string (renderer output or escaped fallback)
//   - links:   lightweight list of extracted URL strings (best-effort)
//
// The returned HTML string should typically be inserted into templates / DOM
// as already-safe content (renderer contracts guarantee escaping / sanitization).
func (s *Service) Render(block core.Block) (htmlOut string, links []string) {
	if block == nil {
		return "<!-- nil block -->", nil
	}

	var rendered template.HTML
	if s.registry != nil {
		rendered = s.registry.Render(block)
	} else {
		// Fallback: escape raw text to avoid accidental injection
		rendered = template.HTML("<pre>" + html.EscapeString(block.Text()) + "</pre>")
	}

	links = ExtractLinks(block.Text())
	return string(rendered), links
}

// BlockHTMLRenderer is a small interface allowing callers to accept either
// *Service or a custom compatible implementation (for testing, instrumentation,
// or alternate rendering strategies).
type BlockHTMLRenderer interface {
	Render(block core.Block) (html string, links []string)
}

// Ensure *Service satisfies BlockHTMLRenderer.
var _ BlockHTMLRenderer = (*Service)(nil)
