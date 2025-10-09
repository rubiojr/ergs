package render

import (
	"html/template"
	"sync"

	"github.com/rubiojr/ergs/pkg/core"
)

/*
Registry & Rendering Service Consolidation

This file contains the unified renderer registry logic originally split across:

  - pkg/renderers/registry.go
  - pkg/renderers/default.go (default / fallback renderer)
  - pkg/renderpipeline (higher-level rendering service – now in service.go)

The goal of consolidation is to:
  * Eliminate duplicate abstractions (registry vs. pipeline service)
  * Provide a single import path: github.com/rubiojr/ergs/pkg/render
  * Allow both the web (SSR) layer and realtime / API layers to use the same
    rendering primitives without import cycles.

Key Concepts:
  - BlockRenderer: Pluggable renderer determined by datasource *type* (or other heuristics).
  - RendererRegistry: Ordered collection of BlockRenderer implementations + a fallback.
  - DefaultRenderer: Generic formatter used when no specific renderer matches.
  - Service (see service.go): High-level convenience for producing HTML + derived metadata.

Thread Safety:
  - The registry uses a RWMutex; reads (Render / GetRenderer / ListRendererTypes / DefaultRenderer)
    are lock-free w.r.t. mutation except for a brief copy of the slice pointer.

Extensibility:
  - Additional renderer discovery mechanisms (e.g. dynamic plugins) can populate the registry.
  - Future optimization (e.g., caching) can be layered on top of Service without changing this API.
*/

// RendererRegistry manages a collection of BlockRenderer implementations plus
// a fallback default renderer used when no specific renderer can handle a block.
type RendererRegistry struct {
	mu              sync.RWMutex
	renderers       []BlockRenderer
	defaultRenderer BlockRenderer
}

// NewRendererRegistry creates an empty registry with a default fallback renderer.
func NewRendererRegistry() *RendererRegistry {
	return &RendererRegistry{
		renderers:       make([]BlockRenderer, 0),
		defaultRenderer: NewDefaultRenderer(),
	}
}

// GetGlobalRegistry builds a registry from all auto‑registered renderers.
// Each call returns a fresh registry snapshot (registration happens via init()).
func GetGlobalRegistry() *RendererRegistry {
	reg := NewRendererRegistry()
	for _, r := range GetRegisteredRenderers() {
		reg.Register(r)
	}
	return reg
}

// Register adds a renderer to the registry.
//
// Safe for concurrent use (lightweight locking).
func (r *RendererRegistry) Register(renderer BlockRenderer) {
	if renderer == nil {
		return
	}
	r.mu.Lock()
	r.renderers = append(r.renderers, renderer)
	r.mu.Unlock()
}

// Render selects the first renderer whose CanRender returns true.
// Falls back to the default renderer if none match.
func (r *RendererRegistry) Render(block core.Block) template.HTML {
	if block == nil {
		return template.HTML("<!-- nil block -->")
	}

	r.mu.RLock()
	renderers := r.renderers
	def := r.defaultRenderer
	r.mu.RUnlock()

	for _, renderer := range renderers {
		if renderer.CanRender(block) {
			return renderer.Render(block)
		}
	}

	if def != nil {
		return def.Render(block)
	}
	return template.HTML("<!-- no renderer available -->")
}

// GetRenderer returns the first matching renderer (without rendering) or nil if none match.
func (r *RendererRegistry) GetRenderer(block core.Block) BlockRenderer {
	if block == nil {
		return nil
	}

	r.mu.RLock()
	defer r.mu.RUnlock()

	for _, renderer := range r.renderers {
		if renderer.CanRender(block) {
			return renderer
		}
	}
	return nil
}

// ListRendererTypes returns the datasource types handled by registered renderers.
// (Primarily useful for debugging / introspection.)
func (r *RendererRegistry) ListRendererTypes() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]string, 0, len(r.renderers))
	seen := make(map[string]struct{})
	for _, ren := range r.renderers {
		t := ren.GetDatasourceType()
		if t == "" {
			// Skip empty type (e.g., default renderer) from the list
			continue
		}
		if _, exists := seen[t]; !exists {
			seen[t] = struct{}{}
			out = append(out, t)
		}
	}
	return out
}

// SetDefaultRenderer overrides the fallback renderer (optional).
func (r *RendererRegistry) SetDefaultRenderer(br BlockRenderer) {
	r.mu.Lock()
	r.defaultRenderer = br
	r.mu.Unlock()
}

// DefaultRenderer returns the currently configured fallback renderer.
func (r *RendererRegistry) DefaultRenderer() BlockRenderer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.defaultRenderer
}
