package rtve

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var rtveTemplate string

// RTVERenderer renders RTVE video/episode blocks
type RTVERenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewRTVERenderer()
	if renderer != nil {
		common.RegisterRenderer(renderer)
	}
}

// NewRTVERenderer creates a new RTVE renderer
func NewRTVERenderer() *RTVERenderer {
	tmpl, err := template.New("rtve").Funcs(common.GetTemplateFuncs()).Parse(rtveTemplate)
	if err != nil {
		return nil
	}

	return &RTVERenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of an RTVE video block
func (r *RTVERenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering RTVE template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from RTVE datasource
func (r *RTVERenderer) CanRender(block core.Block) bool {
	return block.Type() == "rtve"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *RTVERenderer) GetDatasourceType() string {
	return "rtve"
}
