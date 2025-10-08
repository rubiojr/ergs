package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/renderers"
)

//go:embed template.html
var githubTemplate string

// GitHubRenderer renders GitHub event blocks
type GitHubRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewGitHubRenderer()
	if renderer != nil {
		renderers.RegisterRenderer(renderer)
	}
}

// NewGitHubRenderer creates a new GitHub renderer
func NewGitHubRenderer() *GitHubRenderer {
	tmpl, err := template.New("github").Funcs(renderers.GetTemplateFuncs()).Parse(githubTemplate)
	if err != nil {
		return nil
	}

	return &GitHubRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a GitHub event block
func (r *GitHubRenderer) Render(block core.Block) template.HTML {
	data := renderers.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    renderers.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering GitHub template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from GitHub datasource
func (r *GitHubRenderer) CanRender(block core.Block) bool {
	return block.Type() == "github"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *GitHubRenderer) GetDatasourceType() string {
	return "github"
}
