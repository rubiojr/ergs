package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
)

//go:embed template.html
var rssTemplate string

// RSSRenderer renders RSS feed item blocks in a compact style
type RSSRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewRSSRenderer()
	if renderer != nil {
		render.RegisterRenderer(renderer)
	}
}

// NewRSSRenderer creates a new RSS renderer with compact styling
func NewRSSRenderer() *RSSRenderer {
	tmpl, err := template.New("rss").Funcs(render.GetTemplateFuncs()).Parse(rssTemplate)
	if err != nil {
		return nil
	}

	return &RSSRenderer{
		template: tmpl,
	}
}

// Render creates a compact HTML representation of an RSS feed item block
func (r *RSSRenderer) Render(block core.Block) template.HTML {
	data := render.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    render.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering RSS template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from RSS datasource
func (r *RSSRenderer) CanRender(block core.Block) bool {
	return block.Type() == "rss"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *RSSRenderer) GetDatasourceType() string {
	return "rss"
}
