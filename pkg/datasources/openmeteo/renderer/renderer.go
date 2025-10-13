package renderer

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
)

//go:embed template.html
var weatherTemplate string

// WeatherRenderer renders weather data blocks
type WeatherRenderer struct {
	template *template.Template
}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewWeatherRenderer()
	if renderer != nil {
		render.RegisterRenderer(renderer)
	}
}

// NewWeatherRenderer creates a new weather renderer
func NewWeatherRenderer() *WeatherRenderer {
	tmpl, err := template.New("openmeteo").Funcs(render.GetTemplateFuncs()).Parse(weatherTemplate)
	if err != nil {
		return nil
	}

	return &WeatherRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of a weather block
func (r *WeatherRenderer) Render(block core.Block) template.HTML {
	data := render.TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    render.ExtractLinks(block.Text()),
	}

	var buf strings.Builder
	err := r.template.Execute(&buf, data)
	if err != nil {
		return template.HTML("Error rendering weather template")
	}

	return template.HTML(buf.String())
}

// CanRender checks if this block is from openmeteo datasource
func (r *WeatherRenderer) CanRender(block core.Block) bool {
	return block.Type() == "openmeteo"
}

// GetDatasourceType returns the datasource type this renderer handles
func (r *WeatherRenderer) GetDatasourceType() string {
	return "openmeteo"
}
