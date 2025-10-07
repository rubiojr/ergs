# Ergs Web Interface

The Ergs web interface provides a modern, responsive web application for browsing and searching your datasources through a user-friendly interface. It offers an alternative to the command-line interface for interactive data exploration.

## Overview

The web interface provides:
- **Search**: Full-text search across all datasources with pagination
- **Browse**: View and navigate through all your configured datasources
- **Statistics**: Overview of data collection and storage usage
- **Responsive Design**: Works on desktop, tablet, and mobile devices

## Starting the Web Server

```bash
# Start with default settings (localhost:8080)
ergs web

# Custom port and host
ergs web --port 3000 --host 0.0.0.0

# With specific configuration file
ergs web --config /path/to/config.toml --port 8080
```

### Options

- `--port` - Port to listen on (default: 8080)
- `--host` - Host to bind to (default: localhost)
- `--config` - Configuration file path

## Accessing the Interface

Once started, access the web interface at `http://localhost:8080`

### Main Pages

- **Home** (`/`) - Overview of all datasources with statistics
- **Search** (`/search`) - Search across all datasources
- **Datasources** (`/datasources`) - Browse individual datasources
- **API** (`/api/`) - REST API endpoints (see [API Documentation](api.md))

## Using the Web Interface

### Searching

1. Navigate to the search page or use the search box on any page
2. Enter your search terms
3. Results are displayed with 30 items per page
4. Use pagination controls to navigate through results
5. Filter by specific datasources if needed

**Search Tips:**
- Use quotes for exact phrases: `"error message"`
- Search is case-insensitive
- All search terms must be present (AND logic)

### Browsing Datasources

1. Go to the datasources page to see all configured datasources
2. Click on any datasource to view its data
3. Browse through blocks with pagination
4. Each datasource displays data in an appropriate format

### Keyboard Shortcuts

- **Ctrl+K** (or Cmd+K on Mac): Focus search input
- **Escape**: Clear search focus

## Configuration

The web server uses your existing Ergs configuration. Make sure your datasources are properly configured in `~/.config/ergs/config.toml` before starting the web interface.
