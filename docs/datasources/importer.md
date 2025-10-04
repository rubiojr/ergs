# Importer Datasource

The importer datasource provides a way to push blocks from external sources into Ergs through an HTTP API. This is useful for building custom importers that can reuse Ergs datasource code but run externally and push blocks to a central Ergs instance.

## Architecture

The importer system consists of two components:

1. **Importer API Server** (`ergs importer`) - Receives blocks via HTTP POST and stores them in a staging database
2. **Importer Datasource** - A single generic datasource that reads ALL pending blocks from staging and routes them to their target datasources based on block metadata

```
┌─────────────────────────────────────────────────────────────────┐
│ External Importer (Python/Go/Shell script/etc)                  │
│   - Generates blocks with datasource field                      │
│   - POSTs to importer API                                       │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼ POST /api/import/blocks
┌─────────────────────────────────────────────────────────────────┐
│ ergs importer (API Server)                                      │
│   - Receives blocks via HTTP API                                │
│   - Stores in storage_dir/internal/importer.db                  │
│   - Each block contains target datasource                       │
└─────────────────────────────────────────────────────────────────┘
                 │
                 │ Staging Storage
                 ▼
         [importer.db]
                 │
                 │ FetchBlocks() - scheduled pull
                 ▼
┌─────────────────────────────────────────────────────────────────┐
│ Single Importer Datasource (in ergs serve)                      │
│   - Reads ALL pending blocks                                    │
│   - Routes based on block.datasource field                      │
│   - Deletes from staging after successful fetch                 │
└────────────────┬────────────────────────────────────────────────┘
                 │
                 ▼ Sends to warehouse (with correct Source())
┌─────────────────────────────────────────────────────────────────┐
│ Warehouse                                                        │
│   - Routes blocks to datasource-specific databases              │
│   - github-main.db, firefox-main.db, etc.                       │
└─────────────────────────────────────────────────────────────────┘
```

## Use Cases

- **Remote Data Collection**: Run importers on remote machines that push to a central Ergs server
- **External Processing**: Pre-process data with external tools before importing
- **Custom Datasources**: Build datasources in other languages (Python, Ruby, etc.) that push to Ergs
- **Batch Imports**: Import historical data or migrate from other systems
- **Scheduled External Jobs**: Cron jobs or external schedulers that push data periodically

## Configuration

### API Token Setup

The importer system uses API key authentication. Configure in `config.toml`:

```toml
# Importer API configuration
[importer]
api_key = "your-secret-token-here"  # Required for authentication
```

If no token is configured, the importer server will generate a random token on startup and print it to the console. **Copy this token** to persist it across restarts.

### Starting the Importer API Server

```bash
# Start with default settings (localhost:9090)
ergs importer

# Output will show the API key:
# 🔑 API Token: abc123def456...
#
# Copy this token to your config.toml [importer] section

# Custom host and port
ergs importer --host 0.0.0.0 --port 8080

# With custom config file
ergs importer --config /path/to/config.toml --port 9090
```

### Configuring Importer Datasource

Add a **single** importer datasource to your `config.toml`:

```toml
# Importer API configuration (required)
[importer]
api_key = "your-secret-token-here"

# Your native datasources
[datasources.github-main]
type = "github"
interval = "30m0s"
[datasources.github-main.config]
token = "your-token"

[datasources.firefox-main]
type = "firefox"
interval = "1h0m0s"
[datasources.firefox-main.config]
database_path = "/path/to/places.sqlite"

# Single importer datasource
[datasources.importer]
type = "importer"
interval = "5m0s"  # Check for new blocks every 5 minutes
[datasources.importer.config]
api_url = "http://localhost:9090"  # URL of the importer API server
api_key = "your-secret-token-here"  # Must match [importer] api_key
```

**Key Points:**
- Only **one** importer datasource instance is needed
- It fetches ALL pending blocks from staging
- Each block contains a `datasource` field specifying where it should be stored
- The importer routes blocks to the correct datasource automatically
- No per-datasource configuration needed!

### Running the Complete System

You need both the importer API server and the serve command:

```bash
# Terminal 1: Start importer API server
ergs importer --port 9090

# Terminal 2: Start warehouse with importer datasources
ergs serve
```

## API Endpoints

All endpoints (except `/health`) require authentication using a Bearer token.

**Authentication Header:**
```
Authorization: Bearer your-api-token-here
```

### POST /api/import/blocks

Import blocks to the staging database. Each block specifies its target datasource.

**Authentication:** Required

**Request Body:**
```json
{
  "blocks": [
    {
      "id": "unique-block-id",
      "text": "Searchable text content",
      "created_at": "2024-01-15T10:30:00Z",
      "type": "datasource-type",
      "datasource": "github-main",
      "metadata": {
        "key": "value",
        "custom_field": "data"
      }
    }
  ]
}
```

**Response:**
```json
{
  "accepted": 5,
  "rejected": 1,
  "errors": [
    "Block 3 (issue-123): missing text"
  ]
}
```

**Status Codes:**
- `200 OK` - Blocks processed (check response for accepted/rejected counts)
- `400 Bad Request` - Invalid request format or validation errors
- `500 Internal Server Error` - Database or server error

### GET /api/blocks/export

Export and delete all pending blocks. Used internally by the importer datasource.

**Authentication:** Required

**Response:**
```json
{
  "blocks": [
    {
      "id": "unique-block-id",
      "text": "Searchable text content",
      "created_at": "2024-01-15T10:30:00Z",
      "type": "datasource-type",
      "datasource": "github-main",
      "metadata": {}
    }
  ],
  "count": 1
}
```

### GET /health

Health check endpoint.

**Authentication:** Not required

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-15T12:00:00Z",
  "service": "importer"
}
```

### GET /api/stats

Get statistics about pending blocks in the staging database.

**Authentication:** Required

**Response:**
```json
{
  "total_pending_blocks": 150,
  "datasources": {
    "github-backup": {
      "pending_blocks": 100,
      "oldest_block": "2024-01-15T10:00:00Z",
      "newest_block": "2024-01-15T12:00:00Z"
    },
    "firefox-import": {
      "pending_blocks": 50,
      "oldest_block": "2024-01-15T11:00:00Z",
      "newest_block": "2024-01-15T12:00:00Z"
    }
  }
}
```

## Block Format

Each block must include:

- **id** (string, required) - Unique identifier for the block
- **text** (string, required) - Searchable text content
- **created_at** (RFC3339 timestamp, required) - When the block was created
- **type** (string, required) - Datasource type (e.g., "github", "firefox", "custom")
- **datasource** (string, required) - Target datasource name (e.g., "github-main", "firefox-main")
- **metadata** (object, optional) - Additional structured data

## Usage Examples

### Basic Import with cURL

```bash
curl -X POST http://localhost:9090/api/import/blocks \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-token-here" \
  -d '{
    "blocks": [
      {
        "id": "issue-123",
        "text": "Fix critical authentication bug",
        "created_at": "2024-01-15T10:30:00Z",
        "type": "github",
        "datasource": "github-main",
        "metadata": {
          "number": 123,
          "state": "closed",
          "author": "johndoe"
        }
      }
    ]
  }'
```

### Python Importer Example

```python
#!/usr/bin/env python3
import requests
import json
from datetime import datetime

def import_blocks(blocks, api_key):
    url = "http://localhost:9090/api/import/blocks"
    
    headers = {
        "Authorization": f"Bearer {api_key}"
    }
    
    payload = {"blocks": blocks}
    
    response = requests.post(url, json=payload, headers=headers)
    response.raise_for_status()
    
    result = response.json()
    print(f"Accepted: {result['accepted']}, Rejected: {result['rejected']}")
    
    if result.get('errors'):
        print("Errors:")
        for error in result['errors']:
            print(f"  - {error}")

# Example usage
blocks = [
    {
        "id": "custom-001",
        "text": "My custom data",
        "created_at": datetime.utcnow().isoformat() + "Z",
        "type": "custom",
        "datasource": "my-custom-datasource",
        "metadata": {
            "source": "external-system",
            "priority": "high"
        }
    }
]

import_blocks(blocks, "your-api-token-here")
```

### Shell Script Importer

```bash
#!/bin/bash

IMPORTER_URL="http://localhost:9090"

# Create a block
BLOCK=$(cat <<EOF
{
  "blocks": [
    {
      "id": "repo-${RANDOM}",
      "text": "Repository activity",
      "created_at": "$(date -u +%Y-%m-%dT%H:%M:%SZ)",
      "type": "github",
      "datasource": "github-main",
      "metadata": {
        "repository": "user/repo",
        "event": "push"
      }
    }
  ]
}
EOF
)

# Submit to importer
curl -X POST "${IMPORTER_URL}/api/import/blocks" \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer your-api-token-here" \
  -d "$BLOCK"
```

### Go Importer Example

```go
package main

import (
    "bytes"
    "encoding/json"
    "fmt"
    "net/http"
    "time"
)

type Block struct {
    ID         string                 `json:"id"`
    Text       string                 `json:"text"`
    CreatedAt  time.Time              `json:"created_at"`
    Type       string                 `json:"type"`
    Datasource string                 `json:"datasource"`
    Metadata   map[string]interface{} `json:"metadata"`
}

type ImportRequest struct {
    Blocks []Block `json:"blocks"`
}

func importBlocks(blocks []Block, apiToken string) error {
    url := "http://localhost:9090/api/import/blocks"
    
    req := ImportRequest{Blocks: blocks}
    data, err := json.Marshal(req)
    if err != nil {
        return err
    }
    
    httpReq, err := http.NewRequest("POST", url, bytes.NewBuffer(data))
    if err != nil {
        return err
    }
    httpReq.Header.Set("Content-Type", "application/json")
    httpReq.Header.Set("Authorization", "Bearer "+apiToken)
    
    client := &http.Client{}
    resp, err := client.Do(httpReq)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    if resp.StatusCode != http.StatusOK {
        return fmt.Errorf("import failed with status %d", resp.StatusCode)
    }
    
    return nil
}

func main() {
    blocks := []Block{
        {
            ID:         "example-1",
            Text:       "Example block",
            CreatedAt:  time.Now(),
            Type:       "custom",
            Datasource: "my-datasource",
            Metadata: map[string]interface{}{
                "source": "go-importer",
            },
        },
    }
    
    if err := importBlocks(blocks, "your-api-token-here"); err != nil {
        fmt.Printf("Error: %v\n", err)
    }
}
```

## How Datasource Routing Works

The importer uses a simple but powerful routing mechanism:

1. **Blocks specify their target**: Each block includes a `datasource` field
2. **Single importer fetches all**: One importer datasource fetches ALL pending blocks
3. **Routing based on metadata**: For each block, `Source()` is set to the `datasource` field value
4. **Warehouse handles storage**: Warehouse routes blocks to the correct database based on `Source()`

**Example Flow:**

```bash
# Config - single importer datasource:
# [datasources.importer]
# type = "importer"
# interval = "5m0s"
#
# [datasources.github-main]
# type = "github"
#
# [datasources.firefox-main]
# type = "firefox"

# POST to /api/import/blocks with mixed blocks:
curl -X POST http://localhost:9090/api/import/blocks -d '{
  "blocks": [
    {"id": "gh-1", "text": "...", "datasource": "github-main", ...},
    {"id": "ff-1", "text": "...", "datasource": "firefox-main", ...},
    {"id": "gh-2", "text": "...", "datasource": "github-main", ...}
  ]
}'

# → All blocks stored in importer.db staging

# Importer datasource fetches (every 5 minutes):
# → Queries ALL blocks from importer.db
# → For gh-1: creates block with Source()='github-main'
# → For ff-1: creates block with Source()='firefox-main'
# → For gh-2: creates block with Source()='github-main'
# → Sends all to warehouse

# Warehouse routes based on Source():
# → gh-1, gh-2 → stored in github-main.db with type='github'
# → ff-1 → stored in firefox-main.db with type='firefox'

# Cleanup:
# → Importer deletes all processed blocks from staging
```

## Best Practices

### Block IDs

- Use deterministic IDs to avoid duplicates
- Include datasource prefix: `github-issue-123`, `firefox-bookmark-456`
- Use hash of content for idempotency: `sha256(content)`

### Batching

- Send blocks in batches of 50-100 for efficiency
- Don't send too large batches (>1000) as it may cause memory issues
- Handle partial failures (check `rejected` count in response)

### Error Handling

- Always check the response for `rejected` blocks
- Log `errors` array for debugging
- Implement retry logic for network failures
- Don't retry validation errors (fix the data instead)

### Monitoring

- Monitor `/api/stats` to check for growing pending block count
- If pending blocks grow, check that:
  - `ergs serve` is running
  - Importer datasource instances are configured correctly
  - Instance names match the datasource names used in POST requests

### Security

- **Protect API keys**: Store tokens securely, don't commit to version control
- **Use strong tokens**: Configure long, random tokens in production
- **Network security**: Run importer API on localhost or private network
- **Reverse proxy**: Use reverse proxy (nginx, caddy) for public access with TLS
- **Validate data**: Sanitize and validate all incoming data
- **Rate limiting**: Consider rate limiting for production deployments
- **Token rotation**: Periodically rotate API keys

## Troubleshooting

### "Authorization required" or "Invalid token" errors

**Problem:** External importer gets 401 Unauthorized response.

**Solution:**
1. Check that the API key in your importer matches the server token
2. Verify the token is included in the `Authorization: Bearer <token>` header
3. Check the importer server logs for the correct token
4. Ensure the token in `[importer]` section matches `[datasources.importer.config]` token

### Blocks not appearing in searches

1. **Check importer API is running:**
   ```bash
   curl http://localhost:9090/health
   ```

2. **Check staging database has blocks:**
   ```bash
   curl http://localhost:9090/api/stats
   ```

3. **Check serve is running with importer datasource:**
   ```bash
   # In config.toml, ensure:
   [datasources.your-datasource-name]
   type = "importer"
   ```

4. **Verify target datasources exist:**
   - Check that datasources referenced in block `datasource` field exist in config
   - Example: if blocks have `"datasource": "github-main"`, ensure `[datasources.github-main]` is configured

### Blocks stuck in staging database

- Check `ergs serve` logs for errors
- Verify datasource instance is configured in config.toml
- Check fetch interval isn't too long
- Manually trigger fetch: restart `ergs serve`

### Import API returning errors

- Validate JSON format
- Ensure all required fields are present
- Check `created_at` is valid RFC3339 format
- Verify `type` field is not empty
- Check database permissions on `storage_dir/internal/`

## Integration with External Systems

The importer system is designed to work with external data sources and processing pipelines:

- **ETL Pipelines**: Extract data with external tools, transform, and import to Ergs
- **Data Migration**: Import historical data from other systems
- **Real-time Feeds**: Stream data from external APIs or message queues
- **Custom Processors**: Pre-process data with specialized tools before importing
- **Multi-language Support**: Write importers in any language with HTTP support

## Performance Considerations

- Staging database is optimized with WAL mode and indexes
- Blocks are deleted after successful import to prevent growth
- Use batching for large imports
- Consider fetch interval based on import frequency
- Monitor pending block count to detect bottlenecks

## See Also

- [Datasource Development Guide](../datasource.md)
- [Architecture Documentation](../architecture.md)
- [API Documentation](../api.md)