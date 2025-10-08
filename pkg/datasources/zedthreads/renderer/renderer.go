package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/renderers"
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
		renderers.RegisterRenderer(renderer)
	}
}

// NewZedThreadsRenderer creates a new Zed Threads renderer
func NewZedThreadsRenderer() *ZedThreadsRenderer {
	tmpl, err := template.New("zedthreads").Funcs(renderers.GetTemplateFuncs()).Parse(zedThreadsTemplate)
	if err != nil {
		return nil
	}

	return &ZedThreadsRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a Zed Threads block
func (r *ZedThreadsRenderer) Render(block core.Block) template.HTML {
	data := renderers.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    renderers.ExtractLinks(block.Text()),
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
