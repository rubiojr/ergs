package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
)

//go:embed template.html
var codebergTemplate string

// CodebergRenderer renders Codeberg event blocks
type CodebergRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewCodebergRenderer()
	if renderer != nil {
		render.RegisterRenderer(renderer)
	}
}

// NewCodebergRenderer creates a new Codeberg renderer
func NewCodebergRenderer() *CodebergRenderer {
	tmpl, err := template.New("codeberg").Funcs(render.GetTemplateFuncs()).Parse(codebergTemplate)
	if err != nil {
		return nil
	}

	return &CodebergRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a Codeberg event block
func (r *CodebergRenderer) Render(block core.Block) template.HTML {
	data := render.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    render.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering Codeberg template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from Codeberg datasource
func (r *CodebergRenderer) CanRender(block core.Block) bool {
	return block.Type() == "codeberg"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *CodebergRenderer) GetDatasourceType() string {
	return "codeberg"
}
