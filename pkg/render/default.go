package render

import (
	"html/template"
	"strings"

	"github.com/rubiojr/ergs/pkg/core"
)

// defaultTemplate is the fallback HTML template used when no specific renderer
// claims a block. It provides a clean, consistent visual presentation including:
//   - Timestamp
//   - Raw (escaped) block text
//   - Extracted links (if any)
//   - Filtered metadata (excluding noisy internal fields)
//
// NOTE: This template deliberately keeps styling self‑contained to avoid
// coupling with outer page styles (still uses CSS variables where available).
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
  border: 1px solid var(--border);
  background: var(--surface);
  border-radius: 6px;
  margin: 0 0 1.25rem;
  font-size: 14px;
  line-height: 1.45;
  box-shadow: 0 1px 2px var(--shadow);
  overflow: hidden;
  transition: background .25s ease, border-color .25s ease;
}
.block-default .bd-header {
  display: flex;
  align-items: center;
  gap: .5rem;
  padding: .5rem .75rem;
  background: var(--surface-alt);
  border-bottom: 1px solid var(--border);
  font-size: 12px;
  color: var(--text-dim);
  transition: background .25s ease;
}
.block-default .bd-header:hover {
  background: var(--surface);
}
.block-default .bd-icon { font-size: 14px; }
.block-default .bd-time { font-variant-numeric: tabular-nums; }
.block-default .bd-body {
  padding: .75rem .85rem;
  white-space: pre-wrap;
  word-break: break-word;
  color: var(--text);
}
.block-default .bd-links {
  padding: 0 .75rem .6rem;
  display: flex;
  flex-wrap: wrap;
  gap: .5rem;
}
.block-default .bd-links a {
  font-size: 12px;
  background: var(--accent-soft);
  color: var(--accent);
  text-decoration: none;
  padding: 2px 8px;
  border-radius: 12px;
  line-height: 1.3;
  border: 1px solid var(--accent-border);
  transition: background .2s ease, color .2s ease, border-color .2s ease;
}
.block-default .bd-links a:hover {
  background: var(--accent);
  color: var(--text);
  border-color: var(--accent);
}
.block-default .bd-meta {
  margin: .35rem .75rem .75rem;
  font-size: 12px;
  color: var(--text-dim);
  background: var(--surface-alt);
  border: 1px solid var(--border);
  border-radius: 4px;
  padding: .25rem .55rem .55rem;
  transition: background .25s ease, border-color .25s ease;
}
.block-default .bd-meta summary {
  cursor: pointer;
  font-weight: 500;
  outline: none;
  padding: .25rem 0;
  color: var(--text);
  transition: color .2s ease;
}
.block-default .bd-meta summary:hover {
  color: var(--accent);
}
.block-default .bd-meta[open] summary {
  border-bottom: 1px solid var(--border);
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
  color: var(--text-dim);
}
.block-default .bd-meta dd {
  margin: 0;
  color: var(--text);
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

// DefaultRenderer provides generic rendering when no specific renderer matches.
type DefaultRenderer struct {
	tmpl *template.Template
}

// NewDefaultRenderer constructs a new default renderer instance.
func NewDefaultRenderer() *DefaultRenderer {
	t, err := template.New("default_renderer").Funcs(GetTemplateFuncs()).Parse(defaultTemplate)
	if err != nil {
		// Fail closed but return a minimal renderer to avoid panics downstream.
		fallback, _ := template.New("fallback").Parse("<pre>{{.}}</pre>")
		return &DefaultRenderer{tmpl: fallback}
	}
	return &DefaultRenderer{tmpl: t}
}

// Render renders any block using the fallback template.
func (r *DefaultRenderer) Render(block core.Block) template.HTML {
	if block == nil {
		return template.HTML("<!-- nil block -->")
	}
	data := TemplateData{
		Block:    block,
		Metadata: block.Metadata(),
		Links:    ExtractLinks(block.Text()),
	}
	var buf strings.Builder
	if err := r.tmpl.Execute(&buf, data); err != nil {
		return template.HTML("<!-- default renderer error -->")
	}
	return template.HTML(buf.String())
}

// CanRender always returns true (catch‑all fallback).
func (r *DefaultRenderer) CanRender(block core.Block) bool { return true }

// GetDatasourceType returns empty string to denote generic applicability.
func (r *DefaultRenderer) GetDatasourceType() string { return "" }
