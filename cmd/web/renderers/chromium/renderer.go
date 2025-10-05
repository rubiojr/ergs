package chromium

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var chromiumTemplate string

// ChromiumRenderer renders Chromium browsing history blocks in a compact style
type ChromiumRenderer struct {
	template *template.Template
}

func init() {
	renderer := NewChromiumRenderer()
	if renderer != nil {
		common.RegisterRenderer(renderer)
	}
}

// NewChromiumRenderer creates a new Chromium renderer with compact styling
func NewChromiumRenderer() *ChromiumRenderer {
	tmpl, err := template.New("chromium").Funcs(common.GetTemplateFuncs()).Parse(chromiumTemplate)
	if err != nil {
		return nil
	}

	return &ChromiumRenderer{
		template: tmpl,
	}
}

// Render creates a compact HTML representation of a Chromium visit block
func (r *ChromiumRenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering Chromium template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from Chromium datasource
func (r *ChromiumRenderer) CanRender(block core.Block) bool {
	return block.Type() == "chromium"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *ChromiumRenderer) GetDatasourceType() string {
	return "chromium"
}
