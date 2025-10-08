package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/renderers"
)

//go:embed template.html
var firefoxTemplate string

// FirefoxRenderer renders Firefox browsing history blocks in a compact style
type FirefoxRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewFirefoxRenderer()
	if renderer != nil {
		renderers.RegisterRenderer(renderer)
	}
}

// NewFirefoxRenderer creates a new Firefox renderer with compact styling
func NewFirefoxRenderer() *FirefoxRenderer {
	tmpl, err := template.New("firefox").Funcs(renderers.GetTemplateFuncs()).Parse(firefoxTemplate)
	if err != nil {
		return nil
	}

	return &FirefoxRenderer{
		template: tmpl,
	}
}

// Render creates a compact HTML representation of a Firefox visit block
func (r *FirefoxRenderer) Render(block core.Block) template.HTML {
	data := renderers.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    renderers.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering Firefox template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from Firefox datasource
func (r *FirefoxRenderer) CanRender(block core.Block) bool {
	return block.Type() == "firefox"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *FirefoxRenderer) GetDatasourceType() string {
	return "firefox"
}
