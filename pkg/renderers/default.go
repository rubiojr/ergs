package renderers

import (
	_ "embed"
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
)

var defaultTemplate = `
<div class="block-default">
  <div class="bd-header">
    <span class="bd-icon">🗂️</span>
    <span class="bd-time">{{formatTime .Block.CreatedAt}}</span>
  </div>

  <div class="bd-body">
    {{htmlEscape .Block.Text}}
  </div>

  {{if .Links}}
  <div class="bd-links">
    {{range .Links}}<a href="{{.}}" target="_blank" rel="noopener noreferrer">{{.}}</a>{{end}}
  </div>
  {{end}}

  {{if .Metadata}}
    {{$exclude := slice "text" "dstype"}}
    {{$filtered := filterMetadata .Metadata $exclude}}
    {{if $filtered}}
    <details class="bd-meta">
      <summary>Metadata</summary>
      <dl>
        {{range $k,$v := $filtered}}
          <dt>{{$k}}</dt><dd>{{$v}}</dd>
        {{end}}
      </dl>
    </details>
    {{end}}
  {{end}}
</div>

<style>
.block-default {
  border: 1px solid #e2e8f0;
  background: #ffffff;
  border-radius: 6px;
  margin: 0 0 1.25rem;
  font-size: 14px;
  line-height: 1.45;
  box-shadow: 0 1px 2px rgba(0,0,0,0.04);
  overflow: hidden;
}
.block-default .bd-header {
  display: flex;
  align-items: center;
  gap: .5rem;
  padding: .5rem .75rem;
  background: #f8fafc;
  border-bottom: 1px solid #e2e8f0;
  font-size: 12px;
  color: #64748b;
}
.block-default .bd-icon { font-size: 14px; }
.block-default .bd-body {
  padding: .75rem .85rem;
  white-space: pre-wrap;
  word-break: break-word;
  color: #334155;
}
.block-default .bd-links {
  padding: 0 .75rem .6rem;
  display: flex;
  flex-wrap: wrap;
  gap: .5rem;
}
.block-default .bd-links a {
  font-size: 12px;
  background: #eff6ff;
  color: #0369a1;
  text-decoration: none;
  padding: 2px 8px;
  border-radius: 12px;
  line-height: 1.3;
  border: 1px solid #dbeafe;
}
.block-default .bd-links a:hover { background: #dbeafe; }
.block-default .bd-meta {
  margin: .35rem .75rem .75rem;
  font-size: 12px;
  color: #475569;
  background: #f1f5f9;
  border: 1px solid #e2e8f0;
  border-radius: 4px;
  padding: .25rem .55rem .55rem;
}
.block-default .bd-meta summary {
  cursor: pointer;
  font-weight: 500;
  outline: none;
  padding: .25rem 0;
}
.block-default .bd-meta[open] summary {
  border-bottom: 1px solid #e2e8f0;
  margin-bottom: .4rem;
}
.block-default .bd-meta dl {
  margin: 0;
  display: grid;
  grid-template-columns: max-content 1fr;
  gap: .25rem .75rem;
}
.block-default .bd-meta dt {
  margin: 0;
  font-weight: 600;
  color: #64748b;
}
.block-default .bd-meta dd {
  margin: 0;
  color: #334155;
  word-break: break-word;
}
@media (max-width:640px){
  .block-default { font-size:13px; }
  .block-default .bd-body { padding:.65rem .7rem; }
  .block-default .bd-links { padding:0 .65rem .5rem; }
  .block-default .bd-meta { margin:.3rem .65rem .65rem; }
}
</style>
`

// DefaultRenderer provides basic rendering for any block type
type DefaultRenderer struct {
	template *template.Template
}

// Note: Default renderer is not auto-registered - it's used as a fallback in the registry

// NewDefaultRenderer creates a new default renderer
func NewDefaultRenderer() *DefaultRenderer {
	tmpl, err := template.New("default").Funcs(GetTemplateFuncs()).Parse(defaultTemplate)
	if err != nil {
		return nil
	}

	return &DefaultRenderer{
		template: tmpl,
	}
}

// Render creates an HTML representation of any block
func (r *DefaultRenderer) Render(block core.Block) template.HTML {
	data := TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    ExtractLinks(block.Text()),
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
