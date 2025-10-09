# Ergs REST API Documentation

The Ergs REST API provides programmatic access to your datasources, enabling search functionality, data retrieval, and system monitoring. The API returns JSON responses and supports advanced search features including full-text search, pagination, and date filtering.

NEW: A real-time WebSocket Firehose endpoint (`/api/firehose/ws`) is now available for streaming newly ingested blocks with minimal latency. See section "WebSocket Firehose (Real-Time Streaming)" below.

## API Endpoints

All API endpoints are available under the `/api` path prefix. The API returns JSON responses and uses HTTP method-specific routing for security.

### Base URL

```
http://localhost:8080/api
```

### Authentication

Currently, no authentication is required for API access.

### CORS Support

The API includes CORS middleware allowing cross-origin requests from any domain with the following configuration:
- **Access-Control-Allow-Origin**: `*`
- **Access-Control-Allow-Methods**: `GET, POST, PUT, DELETE, OPTIONS`
- **Access-Control-Allow-Headers**: `Content-Type, Authorization`

## Endpoints

### 1. List Datasources

Retrieve a list of all configured datasources with their statistics.

**Endpoint:** `GET /api/datasources`

**Parameters:** None

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

**Status Codes:**
- `200` - Success
- `500` - Internal server error

### 2. Get Datasource Blocks

Retrieve blocks from a specific datasource with optional search and pagination.

**Endpoint:** `GET /api/datasources/{name}`

**Path Parameters:**
- `name` (required) - The datasource name

**Query Parameters:**
- `q` (optional) - Search query string for full-text search within the datasource
- `limit` (optional) - Maximum number of results (default: 20, max: 1000)
- `page` (optional) - Page number for pagination (default: 1, max: 10000)
- `start_date` (optional) - Filter blocks created on or after this date (format: YYYY-MM-DD)
- `end_date` (optional) - Filter blocks created on or before this date (format: YYYY-MM-DD, inclusive of entire day)

**Date Filtering:**
- Dates must be in `YYYY-MM-DD` format (e.g., `2024-01-15`)
- `start_date` filters blocks created on or after the specified date at 00:00:00
- `end_date` filters blocks created on or before the specified date at 23:59:59 (entire day is included)
- Both parameters can be used together to specify a date range
- Invalid date formats return HTTP 400 with error details

**Examples:**
```bash
# Get recent blocks from 'github' datasource
GET /api/datasources/github

# Search within a datasource
GET /api/datasources/github?q=bug+fix

# Limit results and paginate
GET /api/datasources/github?q=feature&limit=10&page=2

# Filter by date range
GET /api/datasources/github?start_date=2024-01-01&end_date=2024-01-31

# Combined filtering
GET /api/datasources/github?q=authentication&limit=5&start_date=2024-01-01
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
- `blocks` - Array of block objects (see Block Object Structure)
- `count` - Number of blocks returned on this page
- `query` - The search query used (if any)

**Status Codes:**
- `200` - Success
- `400` - Bad request (invalid parameters, invalid date format, invalid search syntax)
- `404` - Datasource not found
- `405` - Method not allowed (only GET supported)
- `500` - Internal server error

### 3. Search All Datasources

Search across all configured datasources simultaneously with advanced filtering options.

**Endpoint:** `GET /api/search`

**Query Parameters:**
- `q` (required) - Search query string (supports FTS5 full-text search syntax)
- `limit` (optional) - Maximum number of results per page (default: 30, max: 1000)
- `page` (optional) - Page number for pagination (default: 1, max: 10000)
- `start_date` (optional) - Filter blocks created on or after this date (format: YYYY-MM-DD)
- `end_date` (optional) - Filter blocks created on or before this date (format: YYYY-MM-DD, inclusive of entire day)
- `datasource` (optional) - Limit search to specific datasources (can be specified multiple times)

**Search Query Syntax:**
The search uses SQLite FTS5 (Full-Text Search) with the following supported features:

- **Simple terms:** `authentication bug` (finds blocks containing both terms)
- **Exact phrases:** `"exact phrase"` (use double quotes for phrase search)
- **Boolean operators:** `authentication AND security` or `bug OR issue`
- **Exclusion:** `authentication NOT password`
- **Prefix matching:** `auth*` (matches authentication, authorize, etc.)
- **Column search:** `text:"search in text"` (search specific columns)
- **Case insensitive:** Search is not case sensitive
- **Partial matching:** Matches partial words

**Search Limitations:**
- Forward slashes (`/`) are not allowed in search terms
- Single quotes (`'`) should be avoided; use double quotes (`"`) for phrases
- Complex boolean expressions may cause syntax errors
- Special characters like `&`, `@`, `%` may need to be avoided

**Examples:**
```bash
# Basic search across all datasources
GET /api/search?q=authentication

# Search with pagination
GET /api/search?q=bug+fix&page=2&limit=50

# Search specific datasources only
GET /api/search?q=golang&datasource=github&datasource=rss

# Date-filtered search
GET /api/search?q=release&start_date=2024-01-01&end_date=2024-01-31

# Complex search with phrase and boolean operators
GET /api/search?q="security update" AND (critical OR urgent)

# Prefix search
GET /api/search?q=config* AND NOT deprecated
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
  "total_count": 2,
  "page": 1,
  "limit": 30,
  "total_pages": 1,
  "has_more": false
}
```

**Response Fields:**
- `query` - The search query used
- `results` - Object with datasource names as keys, each containing a ListBlocksResponse
- `total_count` - Total number of results returned on this page
- `page` - Current page number
- `limit` - Maximum results per page
- `total_pages` - Estimated total number of pages
- `has_more` - Whether there are more results available on subsequent pages

**Status Codes:**
- `200` - Success
- `400` - Bad request (missing query parameter, invalid search syntax, invalid date format)
- `405` - Method not allowed (only GET supported)
- `500` - Internal server error

### 4. Get Statistics

Retrieve storage statistics for all configured datasources including block counts and storage information.

**Endpoint:** `GET /api/stats`

**Parameters:** None

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

**Response Fields:**
- Per-datasource statistics with block counts and storage sizes
- `total_blocks` - Total blocks across all datasources
- `total_datasources` - Number of configured datasources

**Status Codes:**
- `200` - Success
- `405` - Method not allowed (only GET supported)
- `500` - Internal server error

### 5. Health Check

Simple health check endpoint to verify the service is running and responsive.

**Endpoint:** `GET /health`

**Parameters:** None

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T12:00:00Z",
  "version": "3.1.0"
}
```

**Response Fields:**
- `status` - Service status (always "ok" if endpoint responds)
- `timestamp` - Current server timestamp in ISO 8601 format
- `version` - Current Ergs version

**Status Codes:**
- `200` - Service is healthy
- `405` - Method not allowed (only GET supported)

**Note:** This endpoint is typically used by load balancers, monitoring systems, and container orchestrators to check service availability.

## Block Object Structure

All API endpoints that return blocks use the following standardized structure:

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

**Standard Fields:**
- `id` - Unique identifier for the block within its datasource
- `text` - Main textual content of the block
- `source` - Original source (URL, file path, etc.)
- `created_at` - Creation timestamp in ISO 8601 format
- `metadata` - Key-value pairs with additional datasource-specific information

**Metadata Examples by Datasource Type:**
- **GitHub**: `author`, `repository`, `labels`, `type` (issue/pull_request)
- **Firefox**: `title`, `visit_count`, `last_visit_date`
- **RSS**: `author`, `feed_title`, `categories`
- **Filesystem**: `file_path`, `file_type`, `size`

## Error Responses

All endpoints return consistent error responses with appropriate HTTP status codes:

```json
{
  "error": "error_type",
  "message": "Human readable error message"
}
```

**Common HTTP Status Codes:**
- `200` - Success
- `400` - Bad Request (missing/invalid parameters, invalid date format, search syntax errors)
- `404` - Not Found (datasource doesn't exist, invalid endpoint)
- `405` - Method Not Allowed (unsupported HTTP method)
- `500` - Internal Server Error

**Common Error Types:**

### Search Syntax Errors (HTTP 400)
```json
{
  "error": "Invalid search query",
  "message": "Forward slashes (/) are not allowed in search terms"
}
```

```json
{
  "error": "Invalid search query", 
  "message": "Unmatched single quotes detected. Use double quotes for phrase searches"
}
```

```json
{
  "error": "Invalid search query",
  "message": "Invalid search syntax. Check for special characters or invalid operators"
}
```

### Parameter Validation Errors (HTTP 400)
```json
{
  "error": "Invalid date format",
  "message": "invalid start_date format: parsing time \"invalid-date\" as \"2006-01-02\": cannot parse \"invalid-date\" as \"2006\""
}
```

```json
{
  "error": "Missing query parameter",
  "message": "Query parameter 'q' is required"
}
```

### Resource Not Found (HTTP 404)
```json
{
  "error": "datasource_not_found",
  "message": "Datasource 'invalid-name' does not exist"
}
```

### Method Not Allowed (HTTP 405)
All endpoints only support GET requests. Other HTTP methods will return:
```json
{
  "error": "method_not_allowed",
  "message": "Method POST not allowed"
}
```

## Search Error Handling

The API provides intelligent error handling for common search issues:

**FTS5 Syntax Errors:** The API detects SQLite FTS5 syntax errors and provides user-friendly error messages instead of exposing internal database errors.

**Graceful Degradation:** Invalid search queries return HTTP 400 with helpful guidance rather than HTTP 500 server errors.

**Error Recovery:** The API suggests alternatives for common search mistakes (e.g., using double quotes instead of single quotes).

## Rate Limiting

Currently, no rate limiting is implemented at the application level. For production deployments, consider:

- Placing the API behind a reverse proxy (nginx, Apache) with rate limiting
- Using cloud-based API gateways with built-in rate limiting
- Implementing custom middleware for rate limiting based on IP or API key

## Parameter Validation and Security

The API implements several security measures:

**Input Validation:**
- Datasource names are validated to contain only safe characters (alphanumeric, underscore, hyphen, dot)
- Numeric parameters (page, limit) are bounds-checked to prevent resource exhaustion
- Date parameters use strict parsing to prevent injection attacks

**SQL Injection Prevention:**
- All database queries use parameterized statements
- Search queries are handled safely through SQLite's FTS5 MATCH parameter binding
- User input is never directly concatenated into SQL strings

**Resource Limits:**
- Maximum limit per request: 1000 results
- Maximum page number: 10000
- Automatic capping of excessive values rather than rejection



## Example Usage

### Using curl

```bash
# List all datasources
curl http://localhost:8080/api/datasources

# Search within a specific datasource with error handling
curl -f "http://localhost:8080/api/datasources/github?q=bug&limit=5" || echo "Request failed"

# Search across all datasources with date filtering
curl "http://localhost:8080/api/search?q=authentication&start_date=2024-01-01&end_date=2024-01-31"

# Get system statistics
curl http://localhost:8080/api/stats

# Health check for monitoring
curl http://localhost:8080/health

# Search with pagination
curl "http://localhost:8080/api/search?q=golang&page=2&limit=50"

# Filter by specific datasources
curl "http://localhost:8080/api/search?q=release&datasource=github&datasource=rss"

# Complex search with boolean operators
curl "http://localhost:8080/api/search?q=\"security+update\"+AND+(critical+OR+urgent)"
```

## Performance Considerations

**Pagination Limits:** The API limits results per page to prevent memory exhaustion and ensure responsiveness.

**Database Optimization:** Consider running `ergs optimize` periodically to maintain FTS5 index performance.

**Caching:** Consider adding HTTP caching for API responses if needed for high-traffic deployments.

## Production Deployment

For production environments, consider:

1. **Reverse Proxy:** Use nginx or Apache for SSL termination and API rate limiting
2. **Process Management:** Use systemd, Docker, or PM2 to manage the ergs web process
3. **Monitoring:** Set up health checks using the `/health` endpoint
4. **Logging:** Configure structured logging and log aggregation
5. **Security:** Implement authentication/authorization if needed
6. **Rate Limiting:** Add rate limiting at the proxy level

See the main documentation for datasource configuration details and deployment best practices.

## WebSocket Firehose (Real-Time Streaming)

The WebSocket Firehose provides near real-time delivery of newly ingested blocks across all datasources. It complements the REST endpoints by pushing updates instead of requiring clients to poll.

### Endpoint

`GET /api/firehose/ws`

### Upgrade

Establish a standard WebSocket connection. No authentication is currently required. CORS/Origin checks are permissive (all origins allowed). This may change for hardened deployments.

### Initial Payload

Upon connection the server sends an `init` message containing the most recent slice of blocks (equivalent to the first page of the REST firehose). If the client supplies a `since` query parameter (`?since=<RFC3339 timestamp>`), only blocks with `created_at` strictly greater than that timestamp are included (high‑precision boundary; supersedes date-based `start_date` filtering):

```/dev/null/api_firehose_ws_init.json#L1-1
{
  "type": "init",
  "count": <number_of_blocks>,
  "blocks": [
    {
      "id": "block-id",
      "text": "Block text",
      "source": "datasource_instance_name",
      "created_at": "2025-01-01T12:00:00Z",
      "metadata": {
        "datasource": "datasource_instance_name",
        "...": "..."
      }
    }
  ]
}
```

### Live Update Messages

After the initial snapshot, new blocks are streamed as discrete messages:

```/dev/null/api_firehose_ws_block.json#L1-1
{
  "type": "block",
  "block": {
    "id": "block-id",
    "text": "Block text",
    "source": "datasource_instance_name",
    "created_at": "2025-01-01T12:00:05Z",
    "metadata": {
      "datasource": "datasource_instance_name"
    }
  }
}
```

When batching is necessary (e.g., fallback polling or catch-up), a `block_batch` message may be sent:

```/dev/null/api_firehose_ws_block_batch.json#L1-1
{
  "type": "block_batch",
  "count": 3,
  "blocks": [ ... same block objects ... ]
}
```

### Heartbeats

A heartbeat message is emitted approximately every 30 seconds so clients can detect stale connections:

```/dev/null/api_firehose_ws_heartbeat.json#L1-1
{
  "type": "heartbeat",
  "ts": "2025-01-01T12:05:00.123456789Z"
}
```

Clients should disconnect and reconnect if no heartbeat or block messages are received within 2× the heartbeat interval.

### Error Messages

Recoverable errors (e.g., transient search/poll failure) are sent as:

```/dev/null/api_firehose_ws_error.json#L1-1
{
  "type": "error",
  "error": "poll_failed",
  "info": "details or diagnostic hint"
}
```

### Message Types Summary

| type         | Purpose                                       |
|--------------|-----------------------------------------------|
| init         | Initial snapshot with recent blocks (optionally filtered by `since`) |
| block        | Single newly ingested block (push)            |
| block_batch  | Multiple new blocks (fallback batch)          |
| heartbeat    | Liveness indicator                            |
| error        | Non-fatal error / diagnostic                  |

### Ordering & De-duplication

- Ordering is generally chronological by `created_at`, but interleaving from multiple datasources can cause slight reordering on high throughput.
- Clients should maintain a de-dup key `(source, id)` to avoid duplicates if reconnecting or processing both `block_batch` and `block` messages.
- When reconnecting, clients can compare the latest `created_at` they have with incoming snapshot data and ignore older blocks.

#### The `since` Query Parameter
Clients may pass `?since=<RFC3339>` when opening the WebSocket (e.g. `wss://host/api/firehose/ws?since=2025-01-01T12:34:56Z`).  
Rules:
- Must be a valid RFC3339 timestamp (e.g. `2025-01-01T12:34:56Z` or with nanoseconds).
- Returns only blocks with `created_at` strictly greater than `since`.
- Overrides any lower-bound implied by `start_date` (the WebSocket endpoint ignores `start_date` when `since` is present).
- The server echoes back an updated high-water mark inside subsequent messages (`init` and any `block_batch`) via the `since` field so clients can persist and reuse it on reconnect.
- If no new blocks exist after that boundary, `init.count` may be zero.

Example:
`wss://localhost:8080/api/firehose/ws?since=2025-01-10T09:00:00Z`

If the newest block you receive has:
```
"created_at": "2025-01-10T09:05:12.345678Z"
```
Store `2025-01-10T09:05:12.345678Z` and use it as the next `since` on reconnect.

### Reconnect Strategy

Suggested client behavior:
1. Connect to `/api/firehose/ws`
2. Process `init` → populate cache
3. Stream `block` / `block_batch` messages
4. On socket close:
   - Exponential backoff reconnect
   - Optionally perform a REST call (`/api/firehose`) to fill any gap

### Backfill Beyond Initial Snapshot

If you need more history than provided in `init`, use the REST firehose endpoint with pagination:
`GET /api/firehose?limit=30&page=N`

### When the Realtime Bridge Is Disabled

If the warehouse event bridge (Unix domain socket) is not configured, the server may fall back to periodic polling, sending `block_batch` messages rather than immediate `block` messages. Heartbeats continue to function regardless of the backend mode.

### Future Extensions (Planned / Possible)

- Optional `since` parameter for reduced initial payload
- Authentication / API keys
- Client-driven ack or cursor offsets
- Compression (permessage-deflate)
- Fine-grained filters (e.g., subset of datasources)

### Example Client (JavaScript)

```/dev/null/api_firehose_ws_client.js#L1-1
const ws = new WebSocket("ws://localhost:8080/api/firehose/ws");

const dedup = new Set();

ws.onmessage = (evt) => {
  const msg = JSON.parse(evt.data);
  switch (msg.type) {
    case "init":
      msg.blocks.forEach(handleBlock);
      break;
    case "block":
      handleBlock(msg.block);
      break;
    case "block_batch":
      msg.blocks.forEach(handleBlock);
      break;
    case "heartbeat":
      // Optionally update a lastSeen timestamp
      break;
    case "error":
      console.warn("Firehose error:", msg.error, msg.info);
      break;
  }
};

function handleBlock(b) {
  const key = `${b.source}:${b.id}`;
  if (dedup.has(key)) return;
  dedup.add(key);
  console.log("New block:", b);
}
```

### Operational Notes

- Ensure both processes (warehouse & web) share filesystem permissions for the Unix domain socket when using the real-time bridge.
- If deploying under systemd, consider placing the socket under `/run/ergs/bridge.sock` with an appropriate `RuntimeDirectory=` directive.
- The WebSocket layer is stateless; horizontal scaling requires each web instance to connect to the same bridge or a future broker.

### Security Considerations

Currently open; for production:
- Restrict origins or add token-based auth.
- Limit connection count.
- Consider rate limiting clients that reconnect aggressively.

### Troubleshooting

| Symptom | Possible Cause | Action |
|---------|----------------|--------|
| No block messages after init | Low ingestion or bridge disabled | Verify warehouse running & socket path configured |
| Frequent reconnects | Network or reverse proxy timeout | Ensure idle timeout > heartbeat interval |
| Duplicate blocks | Reconnect overlap | Implement de-dup logic (see above) |
| Missing recent blocks on reconnect | Gap during downtime | Use REST firehose pages to backfill |
