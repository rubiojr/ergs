# Block Renderers

This directory contains a simplified, self-contained renderer system for different datasource types. Each renderer provides HTML display of block metadata and content.

## Architecture

The renderer system is designed for simplicity and maintainability:

- **Self-contained renderers**: Each renderer lives in its own directory with its template
- **Auto-registration**: Renderers automatically register themselves on import
- **Simple interface**: Just implement `Render()`, `CanRender()`, and `GetDatasourceType()`
- **Shared template functions**: Common functionality available to all templates
- **No configuration complexity**: Straightforward template-based rendering

## Directory Structure

```
renderers/
├── README.md                 # This file
├── init.go                   # Auto-imports all renderers
├── renderer.go               # Core interfaces and registry
├── common/
│   └── funcs.go              # Shared template functions and interface
├── github/
│   ├── renderer.go           # GitHub renderer implementation
│   └── template.html         # GitHub HTML template
├── rss/
│   ├── renderer.go           # RSS renderer implementation
│   └── template.html         # RSS HTML template
├── firefox/
│   ├── renderer.go           # Firefox renderer implementation
│   └── template.html         # Firefox HTML template
├── hackernews/
│   ├── renderer.go           # Hacker News renderer implementation
│   └── template.html         # Hacker News HTML template
├── codeberg/
│   ├── renderer.go           # Codeberg renderer implementation
│   └── template.html         # Codeberg HTML template
├── zedthreads/
│   ├── renderer.go           # Zed Threads renderer implementation
│   └── template.html         # Zed Threads HTML template
├── gasstations/
│   ├── renderer.go           # Gas Stations renderer implementation
│   └── template.html         # Gas Stations HTML template
├── timestamp/
│   ├── renderer.go           # Timestamp renderer implementation
│   └── template.html         # Timestamp HTML template
└── default/
    ├── renderer.go           # Default fallback renderer
    └── template.html         # Default HTML template
```

## Using the Renderer System

### Getting the Registry

```go
import "github.com/rubiojr/ergs/cmd/web/renderers"

// Get the global registry with all auto-registered renderers
registry := renderers.GetGlobalRegistry()

// Render a block
html := registry.Render(block)
```

## Creating a New Renderer

### Step 1: Create Directory Structure

```bash
mkdir ergs/cmd/web/renderers/mydatasource
touch ergs/cmd/web/renderers/mydatasource/renderer.go
touch ergs/cmd/web/renderers/mydatasource/template.html
```

### Step 2: Implement the Renderer

Create `mydatasource/renderer.go`:

```go
package mydatasource

import (
    _ "embed"
    "html/template"
    "strings"
    
    "github.com/rubiojr/ergs/cmd/web/renderers/common"
    "github.com/rubiojr/ergs/pkg/core"
)

//go:embed template.html
var myTemplate string

type MyDatasourceRenderer struct {
    template *template.Template
}

// Auto-registration
func init() {
    renderer := NewMyDatasourceRenderer()
    if renderer != nil {
        common.RegisterRenderer(renderer)
    }
}

func NewMyDatasourceRenderer() *MyDatasourceRenderer {
    tmpl, err := template.New("mydatasource").Funcs(common.GetTemplateFuncs()).Parse(myTemplate)
    if err != nil {
        return nil
    }

    return &MyDatasourceRenderer{
        template: tmpl,
    }
}

func (r *MyDatasourceRenderer) Render(block core.Block) template.HTML {
    data := common.TemplateData{
        Block:    block,
        Metadata: block.Metadata(),
        Links:    common.ExtractLinks(block.Text()),
    }

    var buf strings.Builder
    err := r.template.Execute(&buf, data)
    if err != nil {
        return template.HTML("Error rendering template")
    }

    return template.HTML(buf.String())
}

func (r *MyDatasourceRenderer) CanRender(block core.Block) bool {
    // Check by source name
    if block.Source() == "mydatasource" {
        return true
    }

    metadata := block.Metadata()
    if metadata == nil {
        return false
    }

    // Check metadata source field
    if source, exists := metadata["source"]; exists {
        if str, ok := source.(string); ok && str == "mydatasource" {
            return true
        }
    }

    // Add custom logic for detecting your datasource
    // Check for specific fields, URL patterns, etc.
    
    return false
}

func (r *MyDatasourceRenderer) GetDatasourceType() string {
    return "mydatasource"
}
```

### Step 3: Create the Template

Create `mydatasource/template.html`:

```html
<div class="block-mydatasource">
    <div class="block-header">
        <span class="block-icon">🔧</span>
        <span class="block-title">
            {{$title := index .Metadata "title"}}
            {{if $title}}{{$title}}{{else}}My Datasource Item{{end}}
        </span>
        <span class="block-separator">•</span>
        <time class="block-time" datetime="{{.Block.CreatedAt.Format "2006-01-02T15:04:05Z07:00"}}">
            {{formatTime .Block.CreatedAt}}
        </time>
    </div>

    <div class="item-info">
        {{$description := index .Metadata "description"}}
        {{if $description}}
        <div class="item-summary">{{$description}}</div>
        {{end}}

        {{/* Metadata section */}}
        {{$excludes := slice "title" "description" "source" "dstype"}}
        {{$filteredMetadata := filterMetadata .Metadata $excludes}}
        {{if $filteredMetadata}}
        <div class="metadata-section">
            <details class="metadata-details">
                <summary>Metadata</summary>
                <dl class="metadata-list">
                    {{range $key, $value := $filteredMetadata}}
                        <dt>{{$key}}</dt>
                        <dd>{{$value}}</dd>
                    {{end}}
                </dl>
            </details>
        </div>
        {{end}}
    </div>
</div>

<style>
.block-mydatasource {
    margin-bottom: 1.5rem;
    padding: 1rem;
    border: 1px solid #e9ecef;
    border-left: 4px solid #007bff;
    border-radius: 6px;
    background: white;
    font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", "Roboto", "Helvetica Neue", Arial, sans-serif;
}

.block-mydatasource .block-header {
    margin-bottom: 0.75rem;
    font-size: 0.875rem;
    color: #6c757d;
    display: flex;
    align-items: center;
    gap: 0.5rem;
    flex-wrap: wrap;
}

.block-mydatasource .block-title {
    color: #007bff;
    font-weight: 500;
}

.block-mydatasource .block-time {
    font-variant-numeric: tabular-nums;
}

.item-info {
    background: #f8f9fa;
    border-radius: 6px;
    padding: 0.75rem;
}

.item-summary {
    color: #495057;
    font-size: 0.875rem;
    line-height: 1.6;
    margin-bottom: 0.75rem;
}

.metadata-section {
    margin-top: 0.75rem;
    border-top: 1px solid #e9ecef;
    padding-top: 0.75rem;
}

.metadata-details summary {
    cursor: pointer;
    color: #6c757d;
    font-weight: 500;
    font-size: 0.875rem;
}

.metadata-list {
    margin: 0.5rem 0 0 0;
    display: grid;
    grid-template-columns: auto 1fr;
    gap: 0.25rem 0.75rem;
    padding: 0.75rem;
    background: #f8f9fa;
    border: 1px solid #e9ecef;
    border-radius: 4px;
}

.metadata-list dt {
    font-weight: 500;
    color: #6c757d;
    margin: 0;
    font-size: 0.825rem;
}

.metadata-list dd {
    margin: 0;
    color: #495057;
    word-break: break-word;
    font-size: 0.825rem;
}
</style>
```

### Step 4: Register the Import

Add to `init.go`:

```go
import (
    _ "github.com/rubiojr/ergs/cmd/web/renderers/mydatasource"
    // ... other imports
)
```

That's it! The renderer will automatically register itself and be available for use.

## Template System

### Available Template Functions

All templates have access to these helper functions from `common.GetTemplateFuncs()`:

#### Time & Formatting
- `formatTime` - Human-readable time (e.g., "2 hours ago")
- `truncate` - Truncate text to specified length
- `htmlEscape` - Escape HTML characters
- `safeHTML` - Mark string as safe HTML

#### Metadata Helpers
- `index` - Get value from map: `{{index .Metadata "field_name"}}`
- `filterMetadata` - Filter metadata excluding specified fields

#### Logic Functions
- `and/or` - Logical operations
- `ne/eq` - Equality comparisons
- `gt/lt` - Numeric comparisons
- `default` - Provide default value if nil/empty

#### String Functions
- `upper/lower/title` - Case conversion
- `contains/hasPrefix/hasSuffix` - String testing
- `replace` - String replacement
- `split/trim/join` - String manipulation

#### Slice Functions
- `slice` - Create slice from values: `{{$excludes := slice "field1" "field2"}}`

### Template Data Structure

Templates receive a `common.TemplateData` struct:

```go
type TemplateData struct {
    Block    core.Block                 // The block being rendered
    Metadata map[string]interface{}     // Block metadata
    Links    []string                   // Extracted links from text
}
```

## Renderer Interface

Each renderer must implement the `common.BlockRenderer` interface:

```go
type BlockRenderer interface {
    // Render takes a block and returns formatted HTML for display
    Render(block core.Block) template.HTML

    // CanRender checks if this renderer can handle the given block type
    CanRender(block core.Block) bool

    // GetDatasourceType returns the datasource type this renderer handles
    GetDatasourceType() string
}
```

## Common Patterns

### Datasource Detection

```go
func (r *MyRenderer) CanRender(block core.Block) bool {
    // Check by source name
    if block.Source() == "mydatasource" {
        return true
    }

    metadata := block.Metadata()
    if metadata == nil {
        return false
    }

    // Check metadata source field
    if source, exists := metadata["source"]; exists {
        if str, ok := source.(string); ok && str == "mydatasource" {
            return true
        }
    }

    // Check for specific fields
    requiredFields := []string{"field1", "field2"}
    for _, field := range requiredFields {
        if _, exists := metadata[field]; exists {
            return true
        }
    }

    return false
}
```

### Template Patterns

#### Conditional Content
```html
{{$title := index .Metadata "title"}}
{{if $title}}
    <div class="item-title">{{$title}}</div>
{{end}}
```

#### External Links
```html
{{$url := index .Metadata "url"}}
{{if $url}}
    <a href="{{$url}}" target="_blank" rel="noopener">View Original</a>
{{end}}
```

#### Filtered Metadata
```html
{{$excludes := slice "title" "url" "description" "source" "dstype"}}
{{$filteredMetadata := filterMetadata .Metadata $excludes}}
{{if $filteredMetadata}}
    <div class="metadata-section">
        <details>
            <summary>Metadata</summary>
            <dl>
                {{range $key, $value := $filteredMetadata}}
                    <dt>{{$key}}</dt>
                    <dd>{{$value}}</dd>
                {{end}}
            </dl>
        </details>
    </div>
{{end}}
```

## Best Practices

1. **Keep it simple**: Avoid complex configuration - just implement the interface
2. **Auto-register**: Always include `init()` function for auto-registration
3. **Handle missing data gracefully**: Use conditional templates
4. **Include metadata section**: Show filtered metadata for transparency
5. **Follow naming conventions**: Use datasource name for directories and packages
6. **Test with real data**: Verify with actual blocks from datasource
7. **Use consistent styling**: Follow the same CSS patterns as existing renderers

## Testing

Build and test:

```bash
go build --tags fts5 -o ergs
./ergs serve
```

Test with curl:

```bash
# Check specific datasource
curl -s "http://localhost:8080/datasource/mydatasource" | grep "block-mydatasource"

# Check search results  
curl -s "http://localhost:8080/search?q=test" | grep "block-mydatasource"
```

## System Overview

The simplified system provides:

- ✅ **Self-contained renderers**: Each renderer lives in its own directory
- ✅ **Auto-registration**: Renderers register themselves automatically via `init()` functions  
- ✅ **Simple interface**: Just implement 3 methods
- ✅ **Shared functionality**: Common template functions and utilities in `common` package
- ✅ **No configuration complexity**: Straightforward template-based rendering
- ✅ **No import cycles**: Clean separation between renderer packages and main package

The system is easy to understand, maintain, and extend while providing all the functionality needed for rich block rendering.

## Final Structure

The simplified renderer system consists of just 12 Go files organized as follows:

```
ergs/cmd/web/renderers/
├── README.md           # Documentation
├── init.go            # Auto-imports all renderers (3 lines)
├── renderer.go        # Core registry logic (67 lines)
├── common/
│   └── funcs.go       # Shared functions and interface (179 lines)
└── [8 renderer directories]/
    ├── renderer.go    # Simple implementation (~120 lines each)
    └── template.html  # HTML template
```

**Key Benefits Achieved:**

- **Zero configuration**: No complex setup or builder patterns
- **Auto-registration**: Import the package and renderers register themselves  
- **No import cycles**: Clean separation using common package
- **Self-contained**: Each renderer is completely independent
- **Minimal code**: Simple interface with just 3 required methods
- **Easy testing**: Just drop in template files and implement interface
- **Future-proof**: Adding new renderers requires no changes to existing code

The refactoring reduced complexity while maintaining all functionality, making the system much more maintainable for future development.