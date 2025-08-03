package defaultrenderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var defaultTemplate string

// DefaultRenderer provides basic rendering for any block type
type DefaultRenderer struct {
	template *template.Template
}

// Note: Default renderer is not auto-registered - it's used as a fallback in the registry

// NewDefaultRenderer creates a new default renderer
func NewDefaultRenderer() *DefaultRenderer {
	tmpl, err := template.New("default").Funcs(common.GetTemplateFuncs()).Parse(defaultTemplate)
	if err != nil {
		return nil
	}

	return &DefaultRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of any block
func (r *DefaultRenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering default template")
	}

	return template.HTML(buf.String())
}

// CanRender returns true for any block (this is the fallback renderer)
func (r *DefaultRenderer) CanRender(block core.Block) bool {
	return true // Default renderer can handle any block
}

// GetDatasourceType returns empty string since this handles any datasource
func (r *DefaultRenderer) GetDatasourceType() string {
	return "" // Default renderer doesn't have a specific type
}
