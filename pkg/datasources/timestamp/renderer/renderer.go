package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
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
		common.RegisterRenderer(renderer)
	}
}

// NewTimestampRenderer creates a new timestamp renderer
func NewTimestampRenderer() *TimestampRenderer {
	tmpl, err := template.New("timestamp").Funcs(common.GetTemplateFuncs()).Parse(timestampTemplate)
	if err != nil {
		return nil
	}

	return &TimestampRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a timestamp block
func (r *TimestampRenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
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
