package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
)

//go:embed template.html
var haTemplate string

// HomeAssistantRenderer renders Home Assistant event blocks
type HomeAssistantRenderer struct {
	template *template.Template
}

func init() {
	r := NewHomeAssistantRenderer()
	if r != nil {
		render.RegisterRenderer(r)
	}
}

// NewHomeAssistantRenderer creates a new renderer instance
func NewHomeAssistantRenderer() *HomeAssistantRenderer {
	tmpl, err := template.New("homeassistant").Funcs(render.GetTemplateFuncs()).Parse(haTemplate)
	if err != nil {
		return nil
	}
	return &HomeAssistantRenderer{template: tmpl}
}

// CanRender returns true if the block is a Home Assistant block
func (r *HomeAssistantRenderer) CanRender(block core.Block) bool {
	return block.Type() == "homeassistant"
}

// GetDatasourceType returns the datasource type this renderer supports
func (r *HomeAssistantRenderer) GetDatasourceType() string {
	return "homeassistant"
}

// Render renders the block to HTML
func (r *HomeAssistantRenderer) Render(block core.Block) template.HTML {
	data := render.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    render.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	if err := r.template.Execute(&buf, data); err != nil {
		return template.HTML("Error rendering Home Assistant template")
	}
	return template.HTML(buf.String())
}
