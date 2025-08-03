package renderers

import (
	"html/template"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	defaultrenderer "github.com/rubiojr/ergs/cmd/web/renderers/default"
	"github.com/rubiojr/ergs/pkg/core"
)

// BlockRenderer defines the interface for rendering different types of blocks (re-exported for convenience)
type BlockRenderer = common.BlockRenderer

// TemplateData holds data passed to block templates (re-exported for convenience)
type TemplateData = common.TemplateData

// RendererRegistry manages all available block renderers
type RendererRegistry struct {
	renderers       []BlockRenderer
	defaultRenderer BlockRenderer
}

// NewRendererRegistry creates a new empty renderer registry
func NewRendererRegistry() *RendererRegistry {
	return &RendererRegistry{
		renderers:       make([]BlockRenderer, 0),
		defaultRenderer: defaultrenderer.NewDefaultRenderer(),
	}
}

// GetGlobalRegistry returns the global registry with all auto-registered renderers
func GetGlobalRegistry() *RendererRegistry {
	// Get renderers from common package and create registry
	registry := NewRendererRegistry()
	for _, renderer := range common.GetRegisteredRenderers() {
		registry.Register(renderer)
	}
	return registry
}

// Register adds a new renderer to this registry
func (r *RendererRegistry) Register(renderer BlockRenderer) {
	r.renderers = append(r.renderers, renderer)
}

// Render finds the appropriate renderer for a block and renders it
func (r *RendererRegistry) Render(block core.Block) template.HTML {
	for _, renderer := range r.renderers {
		if renderer.CanRender(block) {
			return renderer.Render(block)
		}
	}

	// Use default renderer as fallback
	if r.defaultRenderer != nil {
		return r.defaultRenderer.Render(block)
	}

	return template.HTML("Error: No renderer found for block")
}

// GetRenderer finds the appropriate renderer for a block
func (r *RendererRegistry) GetRenderer(block core.Block) BlockRenderer {
	for _, renderer := range r.renderers {
		if renderer.CanRender(block) {
			return renderer
		}
	}
	return nil
}

// Re-exported functions for convenience
var ExtractLinks = common.ExtractLinks
var FormatTime = common.FormatTime
var GetTemplateFuncs = common.GetTemplateFuncs
