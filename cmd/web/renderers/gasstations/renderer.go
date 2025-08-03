package gasstations

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var gasStationsTemplate string

// GasStationsRenderer renders gas station data blocks
type GasStationsRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewGasStationsRenderer()
	if renderer != nil {
		common.RegisterRenderer(renderer)
	}
}

// NewGasStationsRenderer creates a new gas stations renderer
func NewGasStationsRenderer() *GasStationsRenderer {
	tmpl, err := template.New("gasstations").Funcs(common.GetTemplateFuncs()).Parse(gasStationsTemplate)
	if err != nil {
		return nil
	}

	return &GasStationsRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a gas station block
func (r *GasStationsRenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering gas stations template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from gas stations datasource
func (r *GasStationsRenderer) CanRender(block core.Block) bool {
	return block.Type() == "gasstations"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *GasStationsRenderer) GetDatasourceType() string {
	return "gasstations"
}
