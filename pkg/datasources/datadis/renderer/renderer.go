package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
)

//go:embed template.html
var datadisTemplate string

// DatadisRenderer renders electricity consumption blocks from Datadis
type DatadisRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewDatadisRenderer()
	if renderer != nil {
		render.RegisterRenderer(renderer)
	}
}

// NewDatadisRenderer creates a new Datadis renderer
func NewDatadisRenderer() *DatadisRenderer {
	tmpl, err := template.New("datadis").Funcs(render.GetTemplateFuncs()).Parse(datadisTemplate)
	if err != nil {
		return nil
	}

	return &DatadisRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a Datadis consumption block
func (r *DatadisRenderer) Render(block core.Block) template.HTML {
	data := render.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    render.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering Datadis template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from Datadis datasource
func (r *DatadisRenderer) CanRender(block core.Block) bool {
	return block.Type() == "datadis"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *DatadisRenderer) GetDatasourceType() string {
	return "datadis"
}
