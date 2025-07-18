# Datasource Development Guide

This guide explains how to create new datasources for Ergs. A datasource is responsible for streaming blocks of data from external APIs in real-time and providing self-registration capabilities.

## Available Datasources

For configuration and usage information for existing datasources, see:

- [Firefox Datasource](datasources/firefox.md) - Extract browsing history from Firefox
- [GitHub Datasource](datasources/github.md) - Fetch GitHub activity and events
- [Codeberg Datasource](datasources/codeberg.md) - Fetch Codeberg activity and events
- [Gas Stations Datasource](datasources/gasstations.md) - Fetch Spanish gas station prices and locations

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