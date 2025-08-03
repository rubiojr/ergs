# Ergs Architecture

Ergs is a generic data fetching and indexing system built around four core concepts: **Blocks**, **Datasources**, **Self-Registration**, and **Streaming Warehouse**. This document provides a comprehensive overview of the system's architecture, design decisions, and component interactions.

## Core Concepts

### 1. Block Interface

The `Block` interface is the fundamental abstraction for any piece of data in Ergs:

```go
type Block interface {
    ID() string
    Text() string
    CreatedAt() time.Time
    Source() string
    Metadata() map[string]interface{}
    PrettyText() string
}
```

**Key Properties:**
- **ID()**: Unique identifier for the block
- **Text()**: Full-text searchable content (indexed by FTS5)
- **CreatedAt()**: Timestamp for temporal ordering
- **Source()**: Identifies which datasource created the block
- **Metadata()**: Flexible key-value data for filtering and analysis
- **PrettyText()**: Rich formatted display with emojis and structured metadata

**Design Principles:**
- Each block type implements the interface directly (no base classes)
- Text content is optimized for full-text search
- Metadata supports arbitrary structured data
- Immutable once created

### 2. Datasource Interface

Datasources define how to retrieve blocks from external APIs:

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

**Responsibilities:**
- **Streaming fetching**: Sends blocks through channels for real-time processing
- **Schema definition**: Defines metadata fields for storage optimization
- **Block creation**: Converts API responses into Block implementations
- **Block reconstruction**: Factory methods recreate blocks from stored data
- **Configuration management**: Self-contained configuration handling
- **Error handling**: Manages API failures gracefully

**Design Principles:**
- Self-contained and stateless
- Streaming architecture for real-time processing
- Self-registration via init() functions
- Factory pattern for block reconstruction
- Configuration validation and type safety
- Simple map-based schema definition

### 3. Self-Registration

The registry system uses Go's init() pattern for automatic datasource registration:

```go
// In each datasource package
func init() {
    prototype := &Datasource{}
    core.RegisterDatasourcePrototype("github", prototype)
}

type Registry struct {
    prototypes  map[string]Datasource
    datasources map[string]Datasource
}
```

**Features:**
- Automatic registration via init() functions
- Plugin-like architecture with underscore imports
- Factory pattern for datasource creation
- Multiple instances of the same datasource type
- Thread-safe operations
- No manual registration required in main()

### 4. Streaming Warehouse

The warehouse system orchestrates real-time data streaming and storage:

```go
type Warehouse struct {
    config         Config
    storageManager *storage.Manager
    datasources    []core.Datasource
    blockCh        chan core.Block
    ticker         *time.Ticker
}
```

**Capabilities:**
- Real-time block streaming via channels
- Concurrent fetching from multiple datasources
- Immediate storage as blocks are received
- Configurable fetch and optimization intervals
- SQLite performance optimizations (WAL mode, caching)
- Graceful start/stop lifecycle with context cancellation

## System Architecture

```
┌─────────────────────────────────────────────────────────────────┐
│                    Ergs Streaming System                       │
├─────────────────────────────────────────────────────────────────┤
│                                                                 │
│  ┌─────────────┐    ┌─────────────┐    ┌─────────────┐        │
│  │   GitHub    │    │  Codeberg   │    │   Custom    │        │
│  │ Datasource  │    │ Datasource  │    │ Datasource  │        │
│  │   (init())  │    │   (init())  │    │   (init())  │        │
│  └─────┬───────┘    └─────┬───────┘    └─────┬───────┘        │
│        │stream              │stream             │stream        │
│        ▼                    ▼                   ▼              │
│        └────────────────────┼───────────────────┘              │
│                             │                                  │
│                    ┌─────────▼─────────┐                       │
│                    │    Warehouse      │◄─── chan Block       │
│                    │  (Real-time)      │                      │
│                    └─────────┬─────────┘                      │
│                             │                                 │
│                    ┌─────────▼─────────┐                      │
│                    │ Storage Manager   │                      │
│                    │ (Block Factories) │                      │
│                    └─────────┬─────────┘                      │
│                             │                                 │
│         ┌───────────────────┼───────────────────┐             │
│         │                   │                   │             │
│  ┌──────▼──────┐   ┌────────▼────────┐   ┌──────▼──────┐     │
│  │ github.db   │   │  codeberg.db    │   │  custom.db  │     │
│  │ (WAL mode)  │   │  (WAL mode)     │   │ (WAL mode)  │     │
│  │ ┌─────────┐ │   │  ┌─────────────┐│   │ ┌─────────┐ │     │
│  │ │ blocks  │ │   │  │   blocks    ││   │ │ blocks  │ │     │
│  │ │ fts5    │ │   │  │   fts5      ││   │ │ fts5    │ │     │
│  │ │metadata │ │   │  │  metadata   ││   │ │metadata │ │     │
│  │ └─────────┘ │   │  └─────────────┘│   │ └─────────┘ │     │
│  └─────────────┘   └─────────────────┘   └─────────────┘     │
│                                                               │
└─────────────────────────────────────────────────────────────────┘
```

## Package Structure

```
pkg/
├── core/                   # Core interfaces and types
│   ├── block.go           # Block interface with PrettyText
│   ├── datasource.go      # Streaming datasource interface
│   ├── registry.go        # Self-registration system
│   └── formatter.go       # Metadata formatting utilities
├── storage/               # Storage management
│   ├── generic.go         # SQLite with WAL and performance opts
│   └── manager.go         # Storage manager with block factories
├── warehouse/             # Streaming orchestration
│   └── warehouse.go       # Real-time warehouse system
├── config/                # Configuration management
│   └── config.go          # Generic TOML configuration
└── datasources/           # Self-registering datasources
    ├── github/
    │   ├── datasource.go  # GitHub streaming + block factory
    │   └── blocks.go      # GitHub blocks with pretty formatting
    └── codeberg/
        ├── datasource.go  # Codeberg streaming + block factory
        └── blocks.go      # Codeberg blocks with pretty formatting

# Command line interface
cmd/
├── web/                   # Web interface & API
│   ├── components/        # Templ components for UI
│   │   ├── layout.templ   # Base layout template
│   │   ├── search.templ   # Search interface
│   │   ├── datasources.templ # Datasource listing
│   │   └── types/         # Type definitions
│   ├── renderers/         # Block renderers for web
│   │   ├── github/        # GitHub-specific renderer
│   │   ├── firefox/       # Firefox-specific renderer
│   │   ├── hackernews/    # Hacker News renderer
│   │   └── common/        # Shared template functions
│   └── static/            # Static assets
│       ├── style.css      # Modern responsive CSS
│       └── script.js      # Minimal UI JavaScript
├── api.go                 # REST API server
└── web.go                 # Web interface server

# Root level
├── datasources.go         # Import all datasources with _
└── main.go               # Uses GetGlobalRegistry()
```

## Data Flow

### 1. Initialization
```
Config Load → Global Registry (auto-populated) → Datasource Creation → Config Setting → Warehouse Setup
```

### 2. Streaming Process
```
Warehouse Timer → Concurrent Channel Streaming → Real-time Block Processing → Immediate Storage
```

### 3. Search Process
```
Query → FTS5 Escaping → Storage Search → Block Factory Reconstruction → PrettyText Display
```

### 4. Self-Registration Flow
```
Import datasources.go → init() functions → RegisterDatasourcePrototype() → Global Registry
```

### 5. Web Interface Flow
```
HTTP Request → Router → Handler → Storage Query → Renderer Selection → Templ Template → HTML Response
```

## Web Interface Architecture

The web interface provides a modern, server-side rendered experience built on top of the core storage and registry systems.

### Component Architecture

- **Templ Templates**: Type-safe server-side templates with Go integration
- **Block Renderers**: Specialized renderers for each datasource type (GitHub, Firefox, etc.)
- **Responsive Design**: Mobile-first CSS with modern layouts
- **Minimal JavaScript**: Only essential UI enhancements

### Rendering Pipeline

1. **Request Routing**: HTTP requests mapped to appropriate handlers
2. **Data Retrieval**: Storage queries using the same interfaces as CLI
3. **Renderer Selection**: Automatic selection based on block source/metadata
4. **Template Rendering**: Server-side HTML generation with templ
5. **Asset Serving**: Optimized CSS/JS with caching headers

### Key Features

- **Pagination**: 30 blocks per page with page number navigation
- **Real-time Search**: Full-text search across all datasources
- **Custom Styling**: Each datasource type has unique, compact styling
- **Keyboard Navigation**: Ctrl+K for search focus, arrow keys for pagination
- **Progressive Enhancement**: Works without JavaScript, enhanced with it

## Storage Architecture

### Per-Datasource Databases with Performance Optimization

Each datasource gets its own SQLite database with performance enhancements:
- **WAL Mode**: Better concurrency for read/write operations
- **Memory Caching**: 64MB cache size for faster queries
- **Memory Mapping**: 256MB mmap for efficient file access
- **Isolation**: Schema changes don't affect other datasources
- **Performance**: Smaller, focused databases
- **Maintainability**: Clear data ownership

### Database Schema

Each datasource database contains:

1. **blocks table**: Core block data
```sql
CREATE TABLE blocks (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    source TEXT NOT NULL,
    datasource TEXT NOT NULL,
    metadata TEXT
);
```

2. **blocks_fts table**: FTS5 virtual table with query escaping
```sql
CREATE VIRTUAL TABLE blocks_fts USING fts5(
    text,
    source,
    datasource,
    metadata,
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);
```

3. **fetch_metadata table**: Operational metadata
```sql
CREATE TABLE fetch_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
```

### Block Factory System

The storage system uses block prototypes to reconstruct original block types:
- Each datasource provides a `BlockPrototype()` method that returns a prototype block
- Storage manager registers prototypes by datasource name
- Search results are converted from generic blocks to original types using `Factory()` method on blocks
- Core system automatically handles source metadata - datasources don't need to manage it
- PrettyText formatting is preserved through reconstruction

### FTS5 Integration with Query Escaping

The storage system automatically:
- Creates FTS5 virtual tables for each datasource
- Escapes special characters in search queries (`=`, `<`, `>`, `!`, etc.)
- Wraps problematic queries in double quotes
- Enables Porter stemming for better search recall
- Maintains synchronized indexes with automatic triggers

## Configuration System

### TOML-Based Configuration

Ergs uses TOML for human-readable configuration:

```toml
storage_dir = '/home/user/.local/share/ergs'
fetch_interval = '30m0s'

[datasources.github-rust]
type = 'github'

[datasources.github-rust.config]
token = 'ghp_...'
language = 'rust'
```

### Configuration Loading

1. **Default locations**: `~/.config/ergs/config.toml`
2. **Environment override**: Via `--config` flag
3. **Validation**: Each datasource validates its own config
4. **Type safety**: Structured loading with error handling

## Design Decisions

### 1. Streaming Architecture

**Decision**: Use channels for real-time block streaming instead of batch processing.

**Rationale**:
- Memory efficient - no need to load all blocks in memory
- Real-time storage as blocks are received
- Better responsiveness for large datasets
- Natural backpressure through channel buffering
- Concurrent processing of multiple datasources

### 2. Self-Registration Pattern

**Decision**: Use Go's init() functions for automatic datasource registration.

**Rationale**:
- Plugin-like architecture with minimal main() code
- Standard Go pattern used by database drivers
- No manual factory registration required
- Clean separation between core and datasource packages
- Easy to add new datasources without core changes

### 3. No Base Classes

**Decision**: Each block type implements the Block interface directly rather than inheriting from a base class.

**Rationale**:
- Cleaner, more explicit implementations
- Prevents inheritance complexity
- Allows block types to optimize their storage
- More idiomatic Go design

### 4. Block Factory Pattern

**Decision**: Blocks implement their own Factory() method for reconstruction from generic storage.

**Rationale**:
- Preserves datasource-specific PrettyText formatting
- Type safety when reconstructing from database
- Enables rich display without storing formatting logic
- Clean separation between storage and presentation
- Core system automatically handles source metadata
- Eliminates developer errors from manual source handling

### 5. Autonomous Datasources

**Decision**: Datasources stream blocks and manage their own fetch strategy.

**Rationale**:
- Each API has different pagination/incremental capabilities
- Real-time streaming enables immediate storage
- Allows datasource-specific optimizations
- Reduces coupling between warehouse and datasources

### 6. Per-Datasource Databases with WAL

**Decision**: Each datasource gets its own SQLite database with WAL mode and performance optimizations.

**Rationale**:
- Better concurrent read/write performance
- Schema evolution independence
- Performance isolation with optimized pragmas
- Clearer data ownership
- Simpler backup/restore procedures

### 7. Configuration Self-Management

**Decision**: Each datasource handles its own configuration validation and client creation.

**Rationale**:
- Encapsulates datasource-specific configuration logic
- Enables dynamic reconfiguration (token updates, etc.)
- Type-safe configuration handling
- Reduces coupling with main application

### 8. Map-Based Schema

**Decision**: Datasources define schema as `map[string]any` rather than SQL DDL.

**Rationale**:
- Language-agnostic schema definition
- Storage system controls SQL generation
- Easier to validate and transform
- Supports future non-SQL storage backends

### 9. FTS5 Query Escaping

**Decision**: Automatically escape FTS5 queries with special characters.

**Rationale**:
- Prevents FTS5 syntax errors from user queries
- Transparent handling of problematic characters
- Maintains search functionality for complex queries
- Better user experience with no query restrictions

## Extensibility Points

### 1. New Datasources

Add new datasources by:
- Implementing the `Datasource` interface with streaming
- Creating block types that implement `Block` with `PrettyText()`
- Adding init() function for self-registration
- Implementing `BlockFactory` for reconstruction
- Adding configuration type and validation
- Adding import to `datasources.go`

### 2. Storage Backends

Extend storage by:
- Implementing alternative storage managers
- Supporting block factory registration
- Maintaining streaming interface contracts
- Supporting the schema transformation system
- Implementing performance optimizations

### 3. Search Capabilities

Enhance search by:
- Extending query escaping for other engines
- Adding new FTS5 configurations
- Implementing result ranking algorithms
- Supporting additional search operators
- Enhancing PrettyText formatting

### 4. Warehouse Strategies

Customize data processing by:
- Implementing alternative warehouse systems
- Adding real-time streaming filters
- Integrating with external message queues
- Supporting multiple storage backends simultaneously
- Adding custom optimization schedules

## Performance Considerations

### 1. Streaming Architecture

- Real-time block processing eliminates memory accumulation
- Channel buffering provides natural backpressure
- Concurrent streaming from multiple datasources
- Immediate storage reduces latency
- Error isolation prevents one datasource from blocking others

### 2. SQLite Performance Optimization

- WAL mode for better concurrent access
- 64MB cache size for faster queries
- Memory mapping (256MB) for large databases
- Automatic ANALYZE and optimization scheduling
- Porter stemming for better search recall
- Query escaping prevents FTS5 syntax errors

### 3. Memory Management

- Streaming processing eliminates large in-memory collections
- Channel buffering controls memory usage
- Block factory pattern avoids storing formatting logic
- Bounded result sets prevent memory exhaustion
- Lazy loading of metadata when not needed

### 4. Database Performance

- Per-datasource databases reduce lock contention
- WAL mode with optimized pragmas
- Automatic optimization and WAL checkpointing
- Block factory reconstruction preserves type information
- FTS5 indexes optimized for search performance

## Security Considerations

### 1. API Token Management

- Tokens stored in configuration files (user responsibility)
- No token logging or exposure in error messages
- Support for environment variable overrides

### 2. SQL Injection Prevention

- All queries use parameterized statements
- Schema validation prevents malicious field names
- FTS5 queries are properly escaped

### 3. File System Security

- Respects XDG Base Directory specification
- Creates directories with appropriate permissions
- No temporary file exposure

### 4. Network Security

- HTTPS enforced for all API calls
- Configurable timeouts prevent hanging connections
- User-Agent headers identify the application

This architecture provides a solid foundation for a generic, extensible data fetching and indexing system while maintaining simplicity and performance.