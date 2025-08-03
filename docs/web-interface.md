# Ergs Web Interface Documentation

The Ergs web interface provides a modern, responsive web application for browsing and searching your datasources. Built with server-side rendering using templ templates, it offers a fast and efficient user experience.

## Overview

The web interface consists of:
- **Modern UI**: Clean, responsive design that works on all devices
- **Server-side Rendering**: Fast page loads with templ templates
- **Custom Renderers**: Specialized display for each datasource type
- **Full-text Search**: Search across all datasources with pagination
- **REST API**: JSON endpoints for programmatic access

## Getting Started

### Starting the Web Server

```bash
# Start with default settings (localhost:8080)
ergs web

# Custom port and host
ergs web --port 3000 --host 0.0.0.0

# With specific configuration
ergs web --config /path/to/config.toml --port 8080
```

### Accessing the Interface

Once started, access the web interface at:
- Home: `http://localhost:8080/`
- Search: `http://localhost:8080/search`
- Datasources: `http://localhost:8080/datasources`
- API: `http://localhost:8080/api/`

## Pages and Features

### Home Page (`/`)

- **Overview**: Statistics and quick access to all datasources
- **Recent Activity**: Summary of recent data collection
- **Quick Search**: Jump directly to search functionality
- **Datasource Grid**: Visual overview of all configured datasources

### Search Interface (`/search`)

- **Universal Search**: Search across all datasources simultaneously
- **Filtering**: Filter by specific datasource
- **Pagination**: Navigate through results (30 per page)
- **Result Highlighting**: Clear presentation of search results
- **Custom Rendering**: Each datasource type displays with appropriate styling

**Search Features:**
- Full-text search with SQLite FTS5
- Real-time result counts
- Page navigation with keyboard shortcuts
- Responsive design for mobile devices

### Datasource Browser (`/datasources`)

- **Datasource List**: All configured datasources with statistics
- **Clickable Cards**: Quick access to individual datasources
- **Status Indicators**: Shows data availability and block counts
- **Type Information**: Clear indication of datasource types

### Individual Datasource (`/datasource/{name}`)

- **Browse Mode**: View all blocks from a specific datasource
- **Pagination**: Navigate through large datasets
- **Custom Rendering**: Datasource-specific display formatting
- **Metadata Display**: Expandable metadata for detailed information

## Block Renderers

The web interface includes specialized renderers for different datasource types:

### GitHub Renderer
- **Pull Requests**: Shows title, author, labels, and status
- **Issues**: Displays issue details with proper formatting
- **Repository Links**: Direct links to GitHub resources
- **Metadata**: Author, repository, timestamps, and labels

### Firefox Renderer
- **Browsing History**: Clean display of visited pages
- **Visit Counts**: Shows frequency of visits
- **Page Titles**: Properly formatted page titles
- **URL Display**: Truncated URLs with full links

### Hacker News Renderer
- **Story Layout**: Mimics Hacker News visual style
- **Vote Scores**: Displays points and rankings
- **Comment Counts**: Links to discussion threads
- **Author Information**: Username and timestamps

### RSS Renderer
- **Feed Items**: Article titles and descriptions
- **Publication Dates**: Formatted timestamps
- **Source Links**: Direct links to original articles
- **Feed Information**: Source feed details

### Zed Threads Renderer
- **Conversation Display**: AI conversation threads
- **Model Information**: Shows AI model used
- **Token Counts**: Input/output token statistics
- **Thread Metadata**: Conversation details

### Default Renderer
- **Fallback Display**: For datasources without custom renderers
- **Metadata Focus**: Emphasizes structured data display
- **Generic Formatting**: Clean, readable presentation

## User Interface Features

### Keyboard Navigation

- **Ctrl+K**: Focus search input
- **Escape**: Clear search focus
- **Arrow Keys**: Navigate pagination (when not in form fields)
- **P/N Keys**: Previous/Next page navigation

### Responsive Design

- **Mobile First**: Optimized for small screens
- **Touch Friendly**: Appropriate touch targets
- **Flexible Layouts**: Adapts to different screen sizes
- **Readable Typography**: Optimal text sizing across devices

### Visual Design

- **Modern Aesthetics**: Clean, contemporary interface
- **Consistent Styling**: Unified design language
- **Accessible Colors**: High contrast and readable
- **Loading States**: Visual feedback for user actions

## Technical Architecture

### Server-side Rendering

The web interface uses **templ** for type-safe server-side templates:

```go
// Example templ component
templ SearchPage(data PageData) {
    @Layout(data) {
        <div class="search-container">
            // Search form and results
        </div>
    }
}
```

**Benefits:**
- Type safety at compile time
- Fast rendering performance
- No client-side hydration required
- SEO-friendly content

### Component Structure

```
cmd/web/components/
├── layout.templ        # Base page layout
├── index.templ         # Home page
├── search.templ        # Search interface
├── datasources.templ   # Datasource listing
├── datasource.templ    # Individual datasource
└── types/
    └── types.go        # Type definitions
```

### Rendering Pipeline

1. **HTTP Request**: Router matches URL to handler
2. **Data Fetching**: Query storage using core interfaces
3. **Renderer Selection**: Choose appropriate block renderer
4. **Template Rendering**: Server-side HTML generation
5. **Response**: Send complete HTML to browser

### Static Assets

- **CSS**: Modern responsive styles with CSS Grid/Flexbox
- **JavaScript**: Minimal enhancements for keyboard navigation
- **Caching**: Appropriate headers for performance
- **Optimization**: Minified and optimized for production

## API Integration

The web interface is built on top of REST API endpoints:

### Available Endpoints

- `GET /api/datasources` - List all datasources
- `GET /api/datasources/{name}` - Get blocks from specific datasource
- `GET /api/search` - Search across all datasources
- `GET /api/stats` - Storage statistics
- `GET /health` - Health check

### JavaScript Integration

The web interface uses minimal JavaScript for enhancements:

```javascript
// Example: Search enhancement
document.addEventListener("keydown", function (e) {
    if ((e.ctrlKey || e.metaKey) && e.key === "k") {
        e.preventDefault();
        const searchInput = document.querySelector('input[name="q"]');
        if (searchInput) {
            searchInput.focus();
            searchInput.select();
        }
    }
});
```

## Configuration

### Web Server Options

```bash
ergs web --help
```

**Available flags:**
- `--port`: Port to listen on (default: 8080)
- `--host`: Host to bind to (default: localhost)
- `--config`: Configuration file path

### Datasource Configuration

The web interface automatically displays all configured datasources. Ensure your `config.toml` is properly set up:

```toml
storage_dir = '/home/user/.local/share/ergs'

[datasources.github]
type = 'github'

[datasources.github.config]
token = 'your-github-token'
```

## Performance Considerations

### Server-side Rendering Benefits

- **Fast Initial Load**: Complete HTML sent to browser
- **SEO Friendly**: Search engines can index content
- **Low JavaScript**: Minimal client-side processing
- **Cache Friendly**: Standard HTTP caching works well

### Pagination Strategy

- **Limited Page Size**: 30 blocks per page for optimal performance
- **Efficient Queries**: Uses LIMIT/OFFSET for database queries
- **Memory Management**: Avoids loading large datasets
- **User Experience**: Quick navigation between pages

### Asset Optimization

- **CSS**: Single, optimized stylesheet
- **JavaScript**: Minimal, focused functionality
- **Caching**: Proper headers for static assets
- **Compression**: Gzip compression for text assets

## Development

### Adding New Renderers

To create a custom renderer for a new datasource type:

1. **Create Directory**: `cmd/web/renderers/mydatasource/`
2. **Implement Renderer**: Follow the `BlockRenderer` interface
3. **Add Template**: Create HTML template with styling
4. **Register**: Use `init()` function for auto-registration

### Template Development

Templates use the templ syntax:

```templ
templ MyComponent(data MyData) {
    <div class="my-component">
        <h2>{ data.Title }</h2>
        if data.ShowDetails {
            <p>{ data.Description }</p>
        }
    </div>
}
```

### Building and Testing

```bash
# Generate templ templates
templ generate

# Build with FTS5 support
make build

# Run tests
make test

# Check linting
make lint
```

## Troubleshooting

### Common Issues

**Port Already in Use**
```bash
# Check what's using the port
lsof -i :8080

# Use a different port
ergs web --port 8081
```

**Template Errors**
```bash
# Regenerate templates
templ generate

# Check for syntax errors
make build
```

**Missing Datasources**
- Verify `config.toml` is in the correct location
- Check datasource configuration syntax
- Ensure required tokens/credentials are set

### Debug Mode

Enable debug logging for troubleshooting:

```bash
ergs --debug web --port 8080
```

### Performance Issues

- Check database file sizes in storage directory
- Monitor memory usage during large searches
- Consider pagination settings for large datasets

## Browser Compatibility

### Supported Browsers

- **Chrome/Chromium**: Version 88+
- **Firefox**: Version 85+
- **Safari**: Version 14+
- **Edge**: Version 88+

### Required Features

- CSS Grid and Flexbox support
- ES6 JavaScript features
- CSS Custom Properties
- Modern form input types

## Security Considerations

### Network Access

- **Default Binding**: localhost only by default
- **Public Access**: Use `--host 0.0.0.0` with caution
- **Firewall**: Consider firewall rules for production
- **Reverse Proxy**: Recommended for production deployments

### Data Protection

- **Local Storage**: All data stored locally by default
- **No Authentication**: Built for single-user local access
- **Token Security**: API tokens stored in local configuration
- **File Permissions**: Ensure proper file system permissions

## Future Enhancements

### Planned Features

- **Real-time Updates**: WebSocket integration for live data
- **Advanced Search**: Query builder interface
- **Data Export**: Export search results in various formats
- **Theming**: Customizable color schemes and layouts
- **Authentication**: Optional user authentication system

### Extension Points

- **Custom Renderers**: Plugin system for third-party renderers
- **API Extensions**: Additional API endpoints for specific needs
- **Dashboard Widgets**: Configurable dashboard components
- **Integration**: Webhooks and external service integration

## Support

For issues, questions, or contributions:

- **Documentation**: Check the main Ergs documentation
- **GitHub Issues**: Report bugs or request features
- **Development**: See the development documentation for contributing
- **Configuration**: Refer to datasource-specific documentation