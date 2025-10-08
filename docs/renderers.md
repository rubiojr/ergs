# Block Renderers

This document explains the (web) block renderer system and how to add / maintain renderers now that each datasource’s renderer lives **with the datasource code**.

## Current Layout

Each datasource may provide an optional renderer located at:

```
pkg/datasources/<datasource>/renderer/
    renderer.go
    template.html
```

Core (shared) renderer infrastructure lives at:

```
pkg/renderers/
    funcs.go         # Template helpers, registration hooks, interfaces
    registry.go      # Registry (GetGlobalRegistry, etc.)
    renderer_default.go  # Fallback (generic) renderer with embedded template
```

## Goals

- Keep datasource-specific presentation logic colocated with the datasource implementation.
- Keep a simple “auto‑registration on import” mechanism (each renderer registers itself in its own `init()`).
- Avoid centralized churn when adding/removing datasource renderers.

## Auto‑Registration Flow

1. Each renderer (`pkg/datasources/<name>/renderer`) has an `init()` that calls `renderers.RegisterRenderer(...)`.
2. The renderer package is brought into the build via a (blank) import anywhere (commonly a dedicated imports file or the relevant command).
3. `renderers.GetGlobalRegistry()` builds a fresh registry using all registered renderers plus the default fallback renderer.
4. The web layer uses that registry to render blocks (falling back when no specific renderer can handle a block).

> IMPORTANT: If a renderer package is never imported, its `init()` does not run and its renderer will not be available. Add a blank import list (example below) if needed.

## Renderer Interface

All renderers implement the `renderers.BlockRenderer` interface (defined in `pkg/renderers/funcs.go`):

```go
type BlockRenderer interface {
    Render(block core.Block) template.HTML
    CanRender(block core.Block) bool
    GetDatasourceType() string
}
```

Meaning:
- `CanRender` is used to select a renderer (typically via `block.Type()`).
- `Render` returns safe HTML (`template.HTML`).
- `GetDatasourceType` returns the canonical datasource type (e.g. `"github"`), used mainly for introspection.

## Template Data

Templates receive:

```go
type TemplateData struct {
    Block    core.Block
    Metadata map[string]interface{}
    Links    []string
}
```

## Available Template Functions

From `renderers.GetTemplateFuncs()`:

Category | Functions
---------|----------
Time & Formatting | `formatTime`, `truncate`, `htmlEscape`, `safeHTML`
Logic & Comparison | `and`, `or`, `eq`, `ne`, `gt`, `lt`, `default`
Strings | `upper`, `lower`, `title`, `contains`, `hasPrefix`, `hasSuffix`, `replace`, `split`, `trim`, `join`
Collections | `slice`
Metadata | `index`, `filterMetadata`
Links | `extractLinks`
JSON | `parseJSON`
Misc | `printf`

## Creating a New Renderer

### 1. Directory & Files

Example for datasource type `myds`:

```
pkg/datasources/myds/
    datasource.go
    blocks.go
    renderer/
        renderer.go
        template.html
```

### 2. Implement `renderer.go`

```go
package renderer

import (
    _ "embed"
    "html/template"
    "strings"

    "github.com/rubiojr/ergs/pkg/core"
    "github.com/rubiojr/ergs/pkg/renderers"
)

//go:embed template.html
var myTemplate string

type MyDSRenderer struct {
    template *template.Template
}

func init() {
    if r := NewMyDSRenderer(); r != nil {
        renderers.RegisterRenderer(r)
    }
}

func NewMyDSRenderer() *MyDSRenderer {
    tmpl, err := template.New("myds").Funcs(renderers.GetTemplateFuncs()).Parse(myTemplate)
    if err != nil {
        return nil
    }
    return &MyDSRenderer{template: tmpl}
}

func (r *MyDSRenderer) CanRender(block core.Block) bool {
    return block.Type() == "myds"
}

func (r *MyDSRenderer) GetDatasourceType() string {
    return "myds"
}

func (r *MyDSRenderer) Render(block core.Block) template.HTML {
    data := renderers.TemplateData{
        Block:    block,
        Metadata: block.Metadata(),
        Links:    renderers.ExtractLinks(block.Text()),
    }

    var buf strings.Builder
    if err := r.template.Execute(&buf, data); err != nil {
        return template.HTML("Renderer error")
    }
    return template.HTML(buf.String())
}
```

### 3. Template (`template.html`)

```html
<div class="block-myds">
    {{$title := index .Metadata "title"}}
    <div class="myds-header">
        <span class="myds-icon">🔧</span>
        {{if $title}}<strong class="myds-title">{{$title}}</strong>{{else}}<strong>MyDS Item</strong>{{end}}
        <span class="myds-time">{{formatTime .Block.CreatedAt}}</span>
    </div>

    {{$description := index .Metadata "description"}}
    {{if $description}}
    <div class="myds-description">
        {{truncate $description 400}}
    </div>
    {{end}}

    {{$exclude := slice "title" "description"}}
    {{$filtered := filterMetadata .Metadata $exclude}}
    {{if $filtered}}
    <details class="myds-meta">
        <summary>Metadata</summary>
        <ul>
            {{range $k,$v := $filtered}}
            <li><strong>{{$k}}:</strong> {{$v}}</li>
            {{end}}
        </ul>
    </details>
    {{end}}
</div>

<style>
.block-myds { border:1px solid #e9ecef; background:#fff; border-radius:6px; margin:0 0 1.25rem; font-size:14px; }
.myds-header { display:flex; gap:.5rem; align-items:center; padding:.6rem .75rem; background:#f8f9fa; border-bottom:1px solid #e9ecef; }
.myds-icon { font-size:14px; }
.myds-title { color:#007bff; }
.myds-time { margin-left:auto; font-size:12px; color:#6c757d; }
.myds-description { padding:.75rem; line-height:1.5; color:#495057; }
.myds-meta { padding:.5rem .75rem .75rem; border-top:1px solid #e9ecef; }
.myds-meta summary { cursor:pointer; color:#6c757d; font-size:12px; }
.myds-meta ul { margin:.5rem 0 0; padding-left:1rem; }
.myds-meta li { font-size:12px; margin:.2rem 0; }
</style>
```

### 4. Ensure the Renderer Package Is Imported

Create (or update) a central imports file, e.g.:

```go
// cmd/web/renderer_imports.go (example)
package main

import (
    _ "github.com/rubiojr/ergs/pkg/datasources/myds/renderer"
    // add other renderer imports here
)
```

Or add the blank import lines in an existing file that is always built for the web UI. Without this, the renderer’s `init()` will not run.

That’s it—no further wiring.

## Migration From Legacy Layout

If you find an **old** renderer under `cmd/web/renderers/<name>`:

1. Move its directory to `pkg/datasources/<name>/renderer`.
2. Rename the package to `renderer`.
3. Update imports to use `github.com/rubiojr/ergs/pkg/renderers`.
4. Add a blank import somewhere (see above).
5. Delete the legacy directory.
6. Rebuild with `make build`.

## Styling Guidelines

- Use a small, consistent font (13–14px).
- Root element class: `.block-<datasource>`.
- Scope CSS inside that root to prevent leakage.
- Use `<details>` for large or optional metadata.
- Avoid inline scripts (renderers should remain pure HTML/CSS).

## Best Practices

1. **Fail Safe**: On template error, return a harmless placeholder string.
2. **Minimal Logic**: Favor declarative templates.
3. **Trim Metadata**: Use `filterMetadata` to exclude noisy fields.
4. **Performance**: Parse template once in `init()`.
5. **Accessibility**: Use semantic tags (`summary`, `time`).
6. **Security**: Only emit trusted HTML with `safeHTML`; everything else should be escaped.

## Testing

Manual check:

```bash
make build
./bin/ergs web
# Visit http://localhost:8080/datasource/<your-datasource>
```

Command-line sanity:

```bash
curl -s http://localhost:8080/datasource/<your-datasource> | grep block-<your-datasource>
```

## Troubleshooting

Issue | Likely Cause | Fix
----- | ------------ | ---
Renderer not applied | Not imported / `CanRender` false | Add blank import / fix `CanRender`
Panic / nil access | Unchecked metadata usage | Guard with `{{if}}`
No metadata section | Filter removed all keys | Adjust exclude list
Duplicate styles | Unscoped selectors | Prefix under `.block-<name>`
Unexpected fallback | Renderer `CanRender` never true | Log / inspect block type

## Future Enhancements (Optional Ideas)

- Hot reload (dev mode).
- Theme variants (CSS variables).
- Metrics / debug endpoint listing active renderer types.

---

If you add or adjust a renderer, follow the steps above and keep things lean. Open an issue if a new shared helper belongs in `pkg/renderers`.
