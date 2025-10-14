package renderer

import (
	"bytes"
	"context"
	_ "embed"
	"html/template"

	"github.com/rubiojr/ergs/pkg/core"
	"github.com/rubiojr/ergs/pkg/render"
)

//go:embed weather.css
var weatherCSS string

// WeatherRenderer renders weather data blocks using templ
type WeatherRenderer struct{}

// init function automatically registers this renderer with the global registry
func init() {
	renderer := NewWeatherRenderer()
	if renderer != nil {
		render.RegisterRenderer(renderer)
	}
}

// NewWeatherRenderer creates a new weather renderer
func NewWeatherRenderer() *WeatherRenderer {
	return &WeatherRenderer{}
}

// Render creates an HTML representation of a weather block using templ
func (r *WeatherRenderer) Render(block core.Block) template.HTML {
	var buf bytes.Buffer

	// Inject CSS styles (only once per page load, ideally)
	buf.WriteString("<style>")
	buf.WriteString(weatherCSS)
	buf.WriteString("</style>")

	// Use the templ component to render
	component := WeatherBlock(block)
	err := component.Render(context.Background(), &buf)
	if err != nil {
		return template.HTML("Error rendering weather template: " + err.Error())
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
