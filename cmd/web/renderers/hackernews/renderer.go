package hackernews

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/cmd/web/renderers/common"
	"github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var hackerNewsTemplate string

// HackerNewsRenderer renders Hacker News item blocks
type HackerNewsRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewHackerNewsRenderer()
	if renderer != nil {
		common.RegisterRenderer(renderer)
	}
}

// NewHackerNewsRenderer creates a new Hacker News renderer
func NewHackerNewsRenderer() *HackerNewsRenderer {
	tmpl, err := template.New("hackernews").Funcs(common.GetTemplateFuncs()).Parse(hackerNewsTemplate)
	if err != nil {
		return nil
	}

	return &HackerNewsRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a Hacker News item block
func (r *HackerNewsRenderer) Render(block core.Block) template.HTML {
	data := common.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    common.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering Hacker News template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from Hacker News datasource
func (r *HackerNewsRenderer) CanRender(block core.Block) bool {
	return block.Type() == "hackernews"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *HackerNewsRenderer) GetDatasourceType() string {
	return "hackernews"
}
