# Datasource Development Guide

This guide explains how to create new datasources for Ergs. A datasource is responsible for streaming blocks of data from external APIs in real-time and providing self-registration capabilities.

## Available Datasources

For configuration and usage information for existing datasources, see:

- [Firefox Datasource](datasources/firefox.md) - Extract browsing history from Firefox
- [GitHub Datasource](datasources/github.md) - Fetch GitHub activity and events
- [Codeberg Datasource](datasources/codeberg.md) - Fetch Codeberg activity and events
- [Gas Stations Datasource](datasources/gasstations.md) - Fetch Spanish gas station prices and locations
- [Datadis Datasource](datasources/datadis.md) - Fetch electricity consumption data from Datadis

## Configurable Fetch Intervals

Each datasource can have its own configurable fetch interval when running the scheduler daemon:

```toml
[datasources.github]
type = 'github'
interval = '15m0s'  # Fetch every 15 minutes
[datasources.github.config]
token = 'your-token'

[datasources.firefox]
type = 'firefox'
# No interval specified - will use default 30m
[datasources.firefox.config]
database_path = '/path/to/places.sqlite'

[datasources.codeberg]
type = 'codeberg'
interval = '1h0m0s'  # Fetch every hour
[datasources.codeberg.config]
token = 'your-token'
```

If no interval is specified for a datasource, it will default to 30 minutes. This allows you to optimize fetch frequency based on how often each data source typically has new content available.

### Schema-Only Datasources (Interval 0)

You can set `interval = '0s'` to disable automatic fetching for a datasource while still registering its schema for storage. This is useful when using the importer API to receive blocks from external sources:

```toml
[datasources.chromium]
type = 'chromium'
interval = '0s'  # Disable automatic fetching (schema-only)
[datasources.chromium.config]
database_path = '/tmp/not-used'  # Won't be accessed

[datasources.importer]
type = 'importer'
interval = '5m0s'  # Check for imported blocks
[datasources.importer.config]
api_url = 'http://localhost:9090'
api_key = 'your-token'
```

With `interval = '0s'`:
- ✅ Schema is registered and database is created
- ✅ Blocks can be imported via the importer API
- ✅ No automatic fetching occurs
- ✅ No errors during scheduled fetch operations
- ⚠️  The datasource's `FetchBlocks()` is never called automatically

This is particularly useful for browser history datasources when using external importers to collect data from remote machines.

## Overview

A datasource in Ergs implements the `core.Datasource` interface and provides:

1. **Streaming block fetching** - Streams data through channels for real-time processing
2. **Schema definition** - Defines the structure of metadata fields
3. **Configuration management** - Self-contained configuration handling and validation
4. **Block reconstruction** - Factory methods to recreate blocks from stored data
5. **Self-registration** - Automatic registration via init() functions

## Core Interface

Every datasource must implement the `core.Datasource` interface:

```go
type Datasource interface {
    Name() string
    FetchBlocks(ctx context.Context, blockCh chan<- Block) error
    Schema() map[string]any
    BlockPrototype() Block
    ConfigType() interface{}
    SetConfig(config interface{}) error
    GetConfig() interface{}
    Close() error
}
```

## Block Interface with Factory Method

Blocks must implement the core interface including their own Factory method:

```go
type Block interface {
    ID() string
    Text() string
    CreatedAt() time.Time
    Source() string
    Metadata() map[string]interface{}
    PrettyText() string
    Factory(genericBlock *GenericBlock, source string) Block
}
```

The Factory method enables blocks to reconstruct themselves from database data. The core system automatically handles source metadata - datasource developers don't need to manage it.

## Step-by-Step Guide

### 1. Create Package Structure

Create a new package under `pkg/datasources/`:

```
pkg/datasources/myapi/
├── datasource.go  # Main datasource with self-registration
├── blocks.go      # Block types with PrettyText formatting
└── config.go      # Configuration types (optional separate file)
```

### 2. Define Configuration

Create a configuration struct with TOML tags:

```go
type Config struct {
    Token    string `toml:"token"`
    BaseURL  string `toml:"base_url"`
    Language string `toml:"language"`
}

func (c *Config) Validate() error {
    if c.BaseURL == "" {
        return fmt.Errorf("base_url is required")
    }
    return nil
}
```

### 3. Implement Block Type

Create a block type that implements the `core.Block` interface:

```go
type MyAPIBlock struct {
    id        string
    text      string
    createdAt time.Time
    source    string
    metadata  map[string]interface{}
    
    // Custom fields specific to your API
    author    string
    category  string
    tags      []string
}

func NewMyAPIBlock(id, title, content, author, source string, createdAt time.Time, tags []string) *MyAPIBlock {
    text := fmt.Sprintf("%s %s %s", title, content, author)
    
    // Note: Don't include source in metadata - core system handles it automatically
    metadata := map[string]interface{}{
        "author":   author,
        "category": "general",
        "tags":     strings.Join(tags, ","),
        "title":    title,
    }
    
    return &MyAPIBlock{
        id:        id,
        text:      text,
        createdAt: createdAt,
        source:    source, // Provided by core system
        metadata:  metadata,
        author:    author,
        tags:      tags,
    }
}

// Implement core.Block interface
func (b *MyAPIBlock) ID() string                        { return b.id }
func (b *MyAPIBlock) Text() string                      { return b.text }
func (b *MyAPIBlock) CreatedAt() time.Time              { return b.createdAt }
func (b *MyAPIBlock) Source() string                    { return b.source }
func (b *MyAPIBlock) Metadata() map[string]interface{} { return b.metadata }

func (b *MyAPIBlock) PrettyText() string {
    metadataInfo := core.FormatMetadata(b.metadata)
    return fmt.Sprintf("📝 MyAPI Post by %s\n  ID: %s\n  Time: %s\n  Tags: %s%s",
        b.author, b.id, b.createdAt.Format("2006-01-02 15:04:05"), 
        strings.Join(b.tags, ", "), metadataInfo)
}

// Factory method for reconstruction from database
func (b *MyAPIBlock) Factory(genericBlock *core.GenericBlock, source string) core.Block {
    metadata := genericBlock.Metadata()
    author := getStringFromMetadata(metadata, "author", "unknown")
    tagsStr := getStringFromMetadata(metadata, "tags", "")
    
    var tags []string
    if tagsStr != "" {
        tags = strings.Split(tagsStr, ",")
    }
    
    return &MyAPIBlock{
        id:        genericBlock.ID(),
        text:      genericBlock.Text(),
        createdAt: genericBlock.CreatedAt(),
        source:    source, // Provided by core system
        metadata:  metadata,
        author:    author,
        tags:      tags,
    }
}

// Custom methods
func (b *MyAPIBlock) Author() string   { return b.author }
func (b *MyAPIBlock) Tags() []string   { return b.tags }

// Helper function for safe metadata extraction
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
    if value, exists := metadata[key]; exists {
        if str, ok := value.(string); ok {
            return str
        }
    }
    return defaultValue
}
```

### 4. Implement Datasource with Self-Registration

Create the main datasource implementation with init() registration:

```go
// Self-registration via init()
func init() {
    prototype := &Datasource{}
    core.RegisterDatasourcePrototype("myapi", prototype)
}

type Datasource struct {
    config *Config
    client *http.Client
}

func NewDatasource(config interface{}) (core.Datasource, error) {
    var apiConfig *Config
    if config == nil {
        apiConfig = &Config{}
    } else {
        var ok bool
        apiConfig, ok = config.(*Config)
        if !ok {
            return nil, fmt.Errorf("invalid config type for MyAPI datasource")
        }
    }
    
    return &Datasource{
        config: apiConfig,
        client: &http.Client{Timeout: 30 * time.Second},
    }, nil
}

func (d *Datasource) Name() string {
    return "myapi"
}

func (d *Datasource) Schema() map[string]any {
    return map[string]any{
        "author":   "TEXT",
        "category": "TEXT", 
        "tags":     "TEXT",
        "title":    "TEXT",
        "score":    "INTEGER",
    }
}

func (d *Datasource) BlockPrototype() core.Block {
    return &MyAPIBlock{}
}

func (d *Datasource) ConfigType() interface{} {
    return &Config{}
}

func (d *Datasource) SetConfig(config interface{}) error {
    if cfg, ok := config.(*Config); ok {
        d.config = cfg
        return cfg.Validate()
    }
    return fmt.Errorf("invalid config type for MyAPI datasource")
}

func (d *Datasource) GetConfig() interface{} {
    return d.config
}

func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
    url := fmt.Sprintf("%s/api/posts", d.config.BaseURL)
    req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
    if err != nil {
        return fmt.Errorf("creating request: %w", err)
    }
    
    if d.config.Token != "" {
        req.Header.Set("Authorization", "Bearer "+d.config.Token)
    }
    
    resp, err := d.client.Do(req)
    if err != nil {
        return fmt.Errorf("making request: %w", err)
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("API returned status %d", resp.StatusCode)
    }
    
    var apiResponse struct {
        Posts []struct {
            ID        string    `json:"id"`
            Title     string    `json:"title"`
            Content   string    `json:"content"`
            Author    string    `json:"author"`
            Tags      []string  `json:"tags"`
            CreatedAt time.Time `json:"created_at"`
        } `json:"posts"`
    }
    
    if err := json.NewDecoder(resp.Body).Decode(&apiResponse); err != nil {
        return fmt.Errorf("decoding response: %w", err)
    }
    
    for _, post := range apiResponse.Posts {
        block := NewMyAPIBlock(
            post.ID,
            post.Title,
            post.Content,
            post.Author,
            post.CreatedAt,
            post.Tags,
        )
        
        select {
        case <-ctx.Done():
            return ctx.Err()
        case blockCh <- block:
            // Block sent successfully
        }
    }
    
    return nil
}

func (d *Datasource) Close() error {
    return nil
}

// Factory function for registration
func Factory(config interface{}) (core.Datasource, error) {
    return NewDatasource(config)
}

// Helper functions for safe metadata extraction
func getStringFromMetadata(metadata map[string]interface{}, key, defaultValue string) string {
    if value, exists := metadata[key]; exists {
        if str, ok := value.(string); ok {
            return str
        }
    }
    return defaultValue
}
```

### 5. Register the Datasource

Add your datasource import to `datasources.go` in the root directory:

```go
package main

import (
    // Import all datasource modules to trigger their init() functions
    _ "github.com/rubiojr/ergs/pkg/datasources/codeberg"
    _ "github.com/rubiojr/ergs/pkg/datasources/github"
    _ "github.com/rubiojr/ergs/pkg/datasources/myapi"  // Add your datasource
)
```

That's it! The datasource will automatically register itself when the package is imported. No manual registration needed in `main.go`.

## Best Practices

### Error Handling

- Always handle network errors gracefully
- Log warnings for non-critical issues
- Return meaningful error messages

```go
if err != nil {
    log.Printf("Warning: failed to fetch optional metadata: %v", err)
    // Continue processing...
}
```

### Rate Limiting

- Respect API rate limits
- Implement backoff strategies
- Add delays between requests when appropriate

```go
time.Sleep(100 * time.Millisecond) // Be nice to APIs
```

### Configuration

- Use clear, descriptive field names
- Provide sensible defaults
- Validate configuration in the `Validate()` method

### Block Text Generation

The `Text()` method should return content optimized for full-text search:

```go
func (b *MyAPIBlock) Text() string {
    // Include searchable content
    return fmt.Sprintf("%s %s %s %s", 
        b.title, 
        b.content, 
        b.author,
        strings.Join(b.tags, " "))
}
```

### PrettyText Formatting

The `PrettyText()` method should return rich, user-friendly formatting:

```go
func (b *MyAPIBlock) PrettyText() string {
    metadataInfo := core.FormatMetadata(b.metadata)
    return fmt.Sprintf("📝 MyAPI Post by %s\n  ID: %s\n  Time: %s\n  Tags: %s%s",
        b.author, b.id, b.createdAt.Format("2006-01-02 15:04:05"), 
        strings.Join(b.tags, ", "), metadataInfo)
}
```

### Schema Definition

Define schema fields that will be useful for filtering and analysis:

```go
func (d *Datasource) Schema() map[string]any {
    return map[string]any{
        "author":     "TEXT",      // For filtering by author
        "category":   "TEXT",      // For categorization
        "score":      "INTEGER",   // For ranking/sorting
        "published":  "BOOLEAN",   // For status filtering
        "created_at": "DATETIME",  // For temporal analysis
    }
}
```

## Testing Your Datasource

Create a simple test to verify your datasource works:

```go
func TestMyAPIDatasource(t *testing.T) {
    config := &Config{
        BaseURL: "https://api.example.com",
        Token:   "test-token",
    }
    
    ds, err := NewDatasource(config)
    if err != nil {
        t.Fatalf("Failed to create datasource: %v", err)
    }
    
    ctx := context.Background()
    blockCh := make(chan core.Block, 10)
    var blocks []core.Block
    
    // Collect blocks from channel
    go func() {
        for block := range blockCh {
            blocks = append(blocks, block)
        }
    }()
    
    err = ds.FetchBlocks(ctx, blockCh)
    close(blockCh)
    
    if err != nil {
        t.Fatalf("Failed to fetch blocks: %v", err)
    }
    
    if len(blocks) == 0 {
        t.Error("Expected some blocks to be fetched")
    }
    
    // Verify block implements interface correctly
    block := blocks[0]
    if block.ID() == "" {
        t.Error("Block ID should not be empty")
    }
    if block.Text() == "" {
        t.Error("Block text should not be empty")
    }
    if block.PrettyText() == "" {
        t.Error("Block PrettyText should not be empty")
    }
}
```

## Usage Example

Once implemented, users can add your datasource like this:

```bash
# Configure in TOML (~/.config/ergs/config.toml)
[datasources.my-data]
type = 'myapi'
# Optional: custom fetch interval (defaults to 30m if not specified)
interval = '15m0s'

[datasources.my-data.config]
token = 'your-api-token'
base_url = 'https://api.example.com'
language = 'en'

# Fetch data once (streams in real-time)
./ergs fetch

# Stream data to stdout
./ergs fetch --stream

# Search (displays with PrettyText formatting)
./ergs search --query "search terms"

# Serve continuously with per-datasource intervals
./ergs serve
```

## Advanced Features

### Incremental Fetching

If your API supports it, implement incremental fetching by tracking state:

```go
func (d *Datasource) FetchBlocks(ctx context.Context) ([]core.Block, error) {
    // You could implement your own state tracking mechanism
    // For example, storing last fetch time in a file or using API cursors
    
    // This is just an example - implement according to your API's capabilities
    lastFetch := d.getLastFetchTime() // Your implementation
    
    // Fetch only new items since lastFetch
    url := fmt.Sprintf("%s/api/posts?since=%s", d.config.BaseURL, lastFetch.Format(time.RFC3339))
    
    // ... rest of implementation
}
```

### Pagination with Streaming

Handle paginated APIs with real-time streaming:

```go
func (d *Datasource) FetchBlocks(ctx context.Context, blockCh chan<- core.Block) error {
    page := 1
    
    for {
        hasMore, err := d.fetchPage(ctx, page, blockCh)
        if err != nil {
            return err
        }
        
        if !hasMore {
            break
        }
        
        page++
        
        // Prevent infinite loops
        if page > 100 {
            log.Printf("Warning: reached maximum page limit")
            break
        }
        
        // Check for cancellation between pages
        select {
        case <-ctx.Done():
            return ctx.Err()
        default:
        }
    }
    
    return nil
}

func (d *Datasource) fetchPage(ctx context.Context, page int, blockCh chan<- core.Block) (bool, error) {
    // Fetch page of data
    blocks, hasMore, err := d.getPageData(ctx, page)
    if err != nil {
        return false, err
    }
    
    // Stream blocks immediately as they're processed
    for _, block := range blocks {
        select {
        case <-ctx.Done():
            return false, ctx.Err()
        case blockCh <- block:
            // Block sent successfully
        }
    }
    
    return hasMore, nil
}
```

### Content Filtering

Filter content based on configuration:

```go
func (d *Datasource) shouldInclude(item APIItem) bool {
    if d.config.Language != "" && item.Language != d.config.Language {
        return false
    }
    
    if d.config.MinScore > 0 && item.Score < d.config.MinScore {
        return false
    }
    
    return true
}
```

## Creating a Web Renderer (Optional)

While not required, creating a custom web renderer provides a polished user experience when viewing your datasource's blocks in the web interface. Without a custom renderer, blocks will use the default generic renderer.

### When to Create a Renderer

Create a custom renderer if:
- Your blocks have rich metadata that deserves special formatting
- You want custom styling that matches the data type
- You have links, images, or other media to display
- The default renderer doesn't showcase your data well

### Renderer Structure

Create a `renderer` subdirectory in your datasource package:

```
pkg/datasources/myapi/
├── datasource.go
├── blocks.go
└── renderer/
    ├── renderer.go     # Renderer implementation
    └── template.html   # HTML template
```

### Implementing the Renderer

Create `renderer/renderer.go`:

```go
package renderer

import (
    _ "embed"
    "html/template"
    "strings"

    "github.com/rubiojr/ergs/pkg/core"
    "github.com/rubiojr/ergs/pkg/render"
)

//go:embed template.html
var myAPITemplate string

type MyAPIRenderer struct {
    template *template.Template
}

// init function automatically registers this renderer
func init() {
    renderer := NewMyAPIRenderer()
    if renderer != nil {
        render.RegisterRenderer(renderer)
    }
}

func NewMyAPIRenderer() *MyAPIRenderer {
    tmpl, err := template.New("myapi").Funcs(render.GetTemplateFuncs()).Parse(myAPITemplate)
    if err != nil {
        return nil
    }
    return &MyAPIRenderer{template: tmpl}
}

func (r *MyAPIRenderer) Render(block core.Block) template.HTML {
    data := render.TemplateData{
        Block:    block,
        Metadata: block.Metadata(),
        Links:    render.ExtractLinks(block.Text()),
    }

    var buf strings.Builder
    err := r.template.Execute(&buf, data)
    if err != nil {
        return template.HTML("Error rendering template")
    }
    return template.HTML(buf.String())
}

func (r *MyAPIRenderer) CanRender(block core.Block) bool {
    return block.Type() == "myapi"
}

func (r *MyAPIRenderer) GetDatasourceType() string {
    return "myapi"
}
```

### Creating the HTML Template

Create `renderer/template.html`:

```html
<div class="block-myapi">
    {{$title := index .Metadata "title"}}
    {{$author := index .Metadata "author"}}
    {{$category := index .Metadata "category"}}
    
    <div class="myapi-header">
        <div class="myapi-icon">📝</div>
        <div class="myapi-content">
            <div class="myapi-title">
                {{if $title}}
                <strong>{{$title}}</strong>
                {{else}}
                <strong>MyAPI Item</strong>
                {{end}}
                {{if $author}}
                <span class="myapi-author">by {{$author}}</span>
                {{end}}
            </div>
            
            <div class="myapi-meta">
                <span class="myapi-time">{{formatTime .Block.CreatedAt}}</span>
                {{if $category}}
                <span class="myapi-separator">|</span>
                <span class="myapi-category">{{$category}}</span>
                {{end}}
            </div>
        </div>
    </div>

    {{$excludes := slice "title" "author" "category"}}
    {{$filteredMetadata := filterMetadata .Metadata $excludes}}
    {{if $filteredMetadata}}
    <div class="myapi-extras">
        <details>
            <summary>Show additional data</summary>
            <dl class="myapi-metadata">
                {{range $key, $value := $filteredMetadata}}
                <dt>{{$key}}</dt>
                <dd>{{$value}}</dd>
                {{end}}
            </dl>
        </details>
    </div>
    {{end}}
</div>

<style>
    .block-myapi {
        margin-bottom: 1.5rem;
        border: 1px solid var(--border);
        border-radius: 4px;
        background: var(--surface);
        font-size: 13px;
    }
    
    .myapi-header {
        display: flex;
        gap: 8px;
        padding: 8px;
        background: var(--surface-alt);
        border-bottom: 1px solid var(--border);
    }
    
    .myapi-icon {
        font-size: 20px;
    }
    
    .myapi-content {
        flex: 1;
        min-width: 0;
    }
    
    .myapi-title {
        margin-bottom: 4px;
        font-size: 14px;
        color: var(--text);
    }
    
    .myapi-author {
        color: var(--text-dim);
        font-weight: normal;
        margin-left: 6px;
    }
    
    .myapi-meta {
        color: var(--text-dim);
        font-size: 11px;
        display: flex;
        gap: 6px;
    }
    
    .myapi-separator {
        color: var(--border-alt);
    }
    
    .myapi-extras {
        padding: 8px;
        border-top: 1px solid var(--border);
    }
    
    .myapi-metadata {
        display: grid;
        grid-template-columns: auto 1fr;
        gap: 4px 12px;
        margin: 8px 0 0 0;
        font-size: 12px;
    }
    
    .myapi-metadata dt {
        font-weight: 500;
        color: var(--text-dim);
    }
    
    .myapi-metadata dd {
        margin: 0;
        color: var(--text);
    }
</style>
```

### Template Functions Available

Your templates have access to these functions (from `render.GetTemplateFuncs()`):

**Time & Formatting:**
- `formatTime` - Format time.Time as readable string
- `truncate` - Truncate string to max length
- `htmlEscape` - Escape HTML entities
- `safeHTML` - Mark string as safe HTML

**Logic:**
- `and`, `or` - Logical operations
- `eq`, `ne`, `gt`, `lt` - Comparisons
- `default` - Provide default value if empty

**Strings:**
- `upper`, `lower`, `title` - Case conversion
- `contains`, `hasPrefix`, `hasSuffix` - String matching
- `replace`, `split`, `trim`, `join` - String manipulation

**Collections:**
- `slice` - Create a slice
- `index` - Access map/slice elements
- `filterMetadata` - Filter metadata by excluding keys

**Other:**
- `extractLinks` - Extract URLs from text
- `parseJSON` - Parse JSON strings
- `printf` - Format strings

### Template Data Structure

Templates receive this data:

```go
type TemplateData struct {
    Block    core.Block                  // The block being rendered
    Metadata map[string]interface{}      // Block's metadata
    Links    []string                    // Extracted URLs from block text
}
```

### Registering the Renderer

Add a blank import to `cmd/renderers_imports.go`:

```go
package cmd

import (
    _ "github.com/rubiojr/ergs/pkg/datasources/chromium/renderer"
    _ "github.com/rubiojr/ergs/pkg/datasources/myapi/renderer"  // Add this line
    // ... other renderers
)
```

Without this import, the renderer's `init()` function won't run and your custom renderer won't be registered.

### Styling Guidelines

1. **Use CSS Variables**: Use `var(--border)`, `var(--surface)`, `var(--text)`, etc. for theme compatibility
2. **Scope Styles**: Use a unique class prefix (`.block-myapi`) to avoid conflicts
3. **Responsive**: Test on mobile/narrow screens
4. **Minimal**: Keep styling lightweight and consistent with other renderers
5. **Semantic HTML**: Use appropriate tags (`<details>`, `<summary>`, `<dl>`, etc.)

### Common CSS Variables

- `--border` - Border color
- `--surface` - Background color
- `--surface-alt` - Alternate background
- `--text` - Primary text color
- `--text-dim` - Secondary text color
- `--text-faint` - Very dim text
- `--accent` - Accent/link color
- `--accent-hover` - Accent hover color
- `--accent-soft` - Soft accent background

### Testing Your Renderer

1. Build the project: `make build`
2. Start the web interface: `./bin/ergs web`
3. Visit `http://localhost:8080`
4. Search for blocks from your datasource
5. Verify the custom styling appears correctly

### Renderer Best Practices

1. **Fail Gracefully**: Handle missing metadata with `{{if}}` checks
2. **Filter Metadata**: Use `filterMetadata` to hide internal/duplicate fields
3. **Collapse Details**: Use `<details>` for optional/verbose information
4. **Performance**: Parse template once in `init()`, not per render
5. **Security**: Always escape user content (automatic unless using `safeHTML`)

For more detailed information about the renderer system, see [docs/renderers.md](renderers.md).

## Key Changes from Legacy System

### Self-Registration
- Use `init()` functions instead of manual registration
- Add imports to `datasources.go` instead of modifying main code
- No need to update configuration handling code

### Streaming Architecture  
- `FetchBlocks()` takes a channel parameter instead of returning slices
- Stream blocks immediately as they're processed
- Handle context cancellation properly in streaming loops

### Block Implementation
- Implement `BlockPrototype()` method returning a prototype block
- Add `Factory()` method to your block type for reconstruction from database
- Don't include source in metadata - core system handles it automatically
- Use helper functions for safe metadata extraction in Factory method
- Add `ConfigType()`, `SetConfig()`, and `GetConfig()` methods
- Support nil configuration during initial creation

### Enhanced Display
- Implement `PrettyText()` for rich formatting with emojis
- Use `core.FormatMetadata()` for consistent metadata display
- Provide context-aware formatting for different block types

This guide provides a complete foundation for creating new datasources with the modern streaming architecture. Each datasource can be tailored to the specific API it targets while maintaining consistency with the Ergs system.