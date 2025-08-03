package zedthreads

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var zedThreadsTemplate string

// ZedThreadsRenderer renders Zed Threads conversation blocks
type ZedThreadsRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewZedThreadsRenderer()
	if renderer != nil {
		common.RegisterRenderer(renderer)
	}
}

// NewZedThreadsRenderer creates a new Zed Threads renderer
func NewZedThreadsRenderer() *ZedThreadsRenderer {
	tmpl, err := template.New("zedthreads").Funcs(common.GetTemplateFuncs()).Parse(zedThreadsTemplate)
	if err != nil {
		return nil
	}

	return &ZedThreadsRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a Zed Threads block
func (r *ZedThreadsRenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering Zed Threads template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from Zed Threads datasource
func (r *ZedThreadsRenderer) CanRender(block core.Block) bool {
	return block.Type() == "zedthreads"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *ZedThreadsRenderer) GetDatasourceType() string {
	return "zedthreads"
}
