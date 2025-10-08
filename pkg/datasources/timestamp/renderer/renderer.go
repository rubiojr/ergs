package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/renderers"
)

//go:embed template.html
var timestampTemplate string

// TimestampRenderer renders timestamp/time-based blocks
type TimestampRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewTimestampRenderer()
	if renderer != nil {
		renderers.RegisterRenderer(renderer)
	}
}

// NewTimestampRenderer creates a new timestamp renderer
func NewTimestampRenderer() *TimestampRenderer {
	tmpl, err := template.New("timestamp").Funcs(renderers.GetTemplateFuncs()).Parse(timestampTemplate)
	if err != nil {
		return nil
	}

	return &TimestampRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a timestamp block
func (r *TimestampRenderer) Render(block core.Block) template.HTML {
	data := renderers.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    renderers.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering timestamp template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from timestamp datasource
func (r *TimestampRenderer) CanRender(block core.Block) bool {
	return block.Type() == "timestamp"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *TimestampRenderer) GetDatasourceType() string {
	return "timestamp"
}
