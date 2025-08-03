# Ergs Web Interface & API Documentation

The Ergs web command provides both a modern web interface and REST API endpoints for accessing and searching your datasources. The web interface uses server-side rendering with templ templates for optimal performance.

## Starting the Web Server

```bash
ergs web [options]
```

### Options

- `--port` - Port to listen on (default: 8080)
- `--host` - Host to bind to (default: localhost)
- `--config` - Configuration file path

### Example

```bash
ergs web --port 3000 --host 0.0.0.0
```

## Web Interface

The web interface provides a complete user experience for browsing and searching your data:

- **Home Page** (`/`) - Overview of all datasources with statistics
- **Search Interface** (`/search`) - Full-text search across all datasources with pagination
- **Datasource Browser** (`/datasources`) - List and browse individual datasources
- **Individual Datasource** (`/datasource/{name}`) - Browse blocks from a specific datasource with pagination

### Features

- **Modern UI**: Clean, responsive design built with templ templates
- **Real-time Rendering**: Server-side rendering with custom block renderers for each datasource type
- **Pagination**: Navigate through large result sets with page numbers (30 blocks per page)
- **Search**: Full-text search with query highlighting and result counts
- **Keyboard Navigation**: Ctrl+K to focus search, arrow keys for pagination
- **Mobile Responsive**: Works well on all device sizes

## API Endpoints

All API endpoints are available under the `/api` path prefix. The API returns JSON responses.

### Base URL

```
http://localhost:8080/api
```

### Authentication

Currently, no authentication is required for API access.

## Endpoints

### 1. List Datasources

Retrieve a list of all configured datasources with their statistics.

**Endpoint:** `GET /api/datasources`

**Response:**
```json
{
  "datasources": [
    {
      "name": "github",
      "type": "github",
      "stats": {
        "total_blocks": 150,
        "last_updated": "2024-01-15T10:30:00Z"
      }
    },
    {
      "name": "notes",
      "type": "filesystem", 
      "stats": {
        "total_blocks": 45
      }
    }
  ],
  "count": 2
}
```

**Response Fields:**
- `datasources` - Array of datasource objects
- `count` - Total number of datasources
- `name` - Datasource identifier
- `type` - Datasource type (github, filesystem, etc.)
- `stats` - Statistics object (may vary by datasource)

### 2. Get Datasource Blocks

Retrieve blocks from a specific datasource with optional search and pagination.

**Endpoint:** `GET /api/datasources/{name}`

**Parameters:**
- `q` (optional) - Search query string
- `limit` (optional) - Maximum number of results (default: 30, max: 100)

**Examples:**
```bash
# Get recent blocks from 'github' datasource
GET /api/datasources/github

# Search within a datasource
GET /api/datasources/github?q=bug+fix

# Limit results
GET /api/datasources/github?q=feature&limit=10
```

**Response:**
```json
{
  "datasource": "github",
  "blocks": [
    {
      "id": "abc123",
      "text": "Fixed critical bug in authentication module",
      "source": "https://github.com/user/repo/issues/123",
      "created_at": "2024-01-15T10:30:00Z",
      "metadata": {
        "author": "john.doe",
        "labels": ["bug", "critical"],
        "repository": "user/repo"
      }
    }
  ],
  "count": 1,
  "query": "bug fix"
}
```

**Response Fields:**
- `datasource` - Name of the datasource
- `blocks` - Array of block objects
- `count` - Number of blocks returned
- `query` - The search query used (if any)

### 3. Search All Datasources

Search across all configured datasources simultaneously.

**Endpoint:** `GET /api/search`

**Parameters:**
- `q` (required) - Search query string
- `limit` (optional) - Maximum number of results per datasource (default: 30, max: 100)

**Example:**
```bash
GET /api/search?q=authentication&limit=5
```

**Response:**
```json
{
  "query": "authentication",
  "results": {
    "github": {
      "datasource": "github",
      "blocks": [
        {
          "id": "def456",
          "text": "Implemented OAuth2 authentication",
          "source": "https://github.com/user/repo/pull/456",
          "created_at": "2024-01-14T15:20:00Z",
          "metadata": {
            "author": "jane.smith",
            "type": "pull_request"
          }
        }
      ],
      "count": 1,
      "query": "authentication"
    },
    "notes": {
      "datasource": "notes",
      "blocks": [
        {
          "id": "ghi789",
          "text": "Notes on authentication best practices",
          "source": "/home/user/notes/auth.md",
          "created_at": "2024-01-13T09:15:00Z",
          "metadata": {
            "file_path": "/home/user/notes/auth.md",
            "file_type": "markdown"
          }
        }
      ],
      "count": 1,
      "query": "authentication"
    }
  },
  "total_count": 2
}
```

**Response Fields:**
- `query` - The search query
- `results` - Object with datasource names as keys
- `total_count` - Total number of results across all datasources

### 4. Get Statistics

Retrieve storage statistics for all datasources.

**Endpoint:** `GET /api/stats`

**Response:**
```json
{
  "github": {
    "total_blocks": 150,
    "last_updated": "2024-01-15T10:30:00Z",
    "storage_size": "2.5MB"
  },
  "notes": {
    "total_blocks": 45,
    "storage_size": "1.2MB"
  },
  "total_blocks": 195,
  "total_datasources": 2
}
```

### 5. Health Check

Simple health check endpoint to verify the service is running.

**Endpoint:** `GET /health`

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T12:00:00Z",
  "version": "1.0.0"
}
```

## Block Object Structure

All API endpoints that return blocks use the following structure:

```json
{
  "id": "unique-block-identifier",
  "text": "The main content/text of the block",
  "source": "Source URL or file path",
  "created_at": "2024-01-15T10:30:00Z",
  "metadata": {
    "key": "value",
    "additional": "datasource-specific fields"
  }
}
```

**Fields:**
- `id` - Unique identifier for the block
- `text` - Main textual content
- `source` - Original source (URL, file path, etc.)
- `created_at` - ISO 8601 timestamp
- `metadata` - Key-value pairs with additional information

## Error Responses

All endpoints return consistent error responses:

```json
{
  "error": "error_type",
  "message": "Human readable error message"
}
```

**Common HTTP Status Codes:**
- `200` - Success
- `400` - Bad Request (missing required parameters)
- `404` - Not Found (datasource doesn't exist)
- `405` - Method Not Allowed
- `500` - Internal Server Error

**Example Error:**
```json
{
  "error": "datasource_not_found",
  "message": "Datasource 'invalid-name' does not exist"
}
```

## Search Query Syntax

The search functionality supports full-text search with the following features:

- **Simple terms:** `authentication bug`
- **Exact phrases:** `"exact phrase"`
- **Case insensitive:** Search is not case sensitive
- **Partial matching:** Matches partial words
- **Multiple terms:** All terms must be present (AND logic)

## Rate Limiting

Currently, no rate limiting is implemented. For production deployments, consider placing the API behind a reverse proxy with appropriate rate limiting.

## Rendering System

The web interface uses a sophisticated block rendering system:

- **Custom Renderers**: Each datasource type has a specialized renderer (GitHub, Firefox, RSS, etc.)
- **Templ Templates**: Type-safe server-side templates for optimal performance
- **Consistent Styling**: Modern, compact design across all renderers
- **Metadata Display**: Expandable metadata sections for detailed information
- **Link Extraction**: Automatic detection and formatting of URLs in content

## Static Assets

The web interface serves optimized static assets:

- `GET /static/style.css` - Modern CSS with responsive design
- `GET /static/script.js` - Minimal JavaScript for UI enhancements
- Cached assets with appropriate headers for performance

## Example Usage

### Using curl

```bash
# List all datasources
curl http://localhost:8080/api/datasources

# Search within a specific datasource  
curl "http://localhost:8080/api/datasources/github?q=bug&limit=5"

# Search across all datasources
curl "http://localhost:8080/api/search?q=authentication"

# Get health status
curl http://localhost:8080/health
```

### Using JavaScript

```javascript
// Fetch datasources
const response = await fetch('/api/datasources');
const data = await response.json();
console.log(`Found ${data.count} datasources`);

// Search with error handling
try {
  const searchResponse = await fetch('/api/search?q=bug+fix&limit=30');
  if (!searchResponse.ok) {
    throw new Error(`HTTP ${searchResponse.status}`);
  }
  const results = await searchResponse.json();
  console.log(`Found ${results.total_count} results`);
} catch (error) {
  console.error('Search failed:', error);
}
```

### Using the Web Interface

```bash
# Start the web server
ergs web --port 8080 --host localhost

# Access the interface
open http://localhost:8080

# Or with custom configuration
ergs web --port 3000 --host 0.0.0.0 --config /path/to/config.toml
```

## Configuration

The web server inherits configuration from the main Ergs configuration file. Ensure your datasources are properly configured before starting the web server.

### Performance Notes

- **Server-side Rendering**: All pages are rendered server-side for optimal performance
- **Pagination**: Limited to 30 blocks per page to maintain responsiveness
- **Caching**: Static assets are cached with appropriate headers
- **Minimal JavaScript**: Only essential UI enhancements are included

### Browser Compatibility

The web interface is compatible with modern browsers that support:
- CSS Grid and Flexbox
- ES6 JavaScript features
- CSS Custom Properties (variables)

See the main documentation for datasource configuration details.