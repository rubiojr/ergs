# RTVE Importer Example

This is a standalone importer that reads RTVE video JSON files and their subtitles from disk and imports them into Ergs via the importer API.

## Overview

This example demonstrates how to build external importers that:
1. Read data from files/APIs/databases
2. Convert to Ergs block format
3. Push to the importer API
4. Have blocks stored in their native datasource database

## Architecture

```
rtve-videos/
  └── 2025/
      └── 2025-10-03/
          └── video_16755959.json
                ↓
         RTVE Importer (this tool)
                ↓ POST /api/import/blocks
         Importer API Server
                ↓
         Staging Database
                ↓
         Importer Datasource (ergs serve)
                ↓
         Warehouse
                ↓
         rtve.db (native storage with type="rtve")
```

## Prerequisites

1. **Ergs configured with importer datasource:**

```toml
# ~/.config/ergs/config.toml

[importer]
api_token = "your-api-token-here"

[datasources.rtve]
type = "rtve"
interval = "1h0m0s"
[datasources.rtve.config]
show_id = "telediario-1"

[datasources.importer]
type = "importer"
interval = "5m0s"
[datasources.importer.config]
api_url = "http://localhost:9090"
api_token = "your-api-token-here"
```

2. **RTVE video JSON files in `rtve-videos/` directory**

Expected directory structure:
```
rtve-videos/
└── 2025/
    └── 2025-10-03/
        ├── video_16755959.json
        └── subs/
            ├── 16755959_es.vtt
            ├── 16755959_en.vtt
            ├── 16755959_ca.vtt
            ├── 16755959_eu.vtt
            └── 16755959_gl.vtt
```

Video JSON format:
```json
{
  "uri": "https://www.rtve.es/api/videos/16755959",
  "htmlUrl": "https://www.rtve.es/play/videos/telediario-1/15-horas-03-10-25/16755959/",
  "id": "16755959",
  "longTitle": "Telediario - 15 horas - 03/10/25",
  "publicationDate": "03-10-2025 15:00:00"
}
```

Subtitles are optional. If a `subs/` directory exists alongside the video JSON file, the importer will:
- Load all available subtitle languages
- Parse Spanish subtitles (`.vtt` format) for full-text search
- Convert to JSON format for storage

## Building

```bash
cd examples/rtve-importer
go build -o rtve-importer
```

## Usage

### Step 1: Start Importer API Server

```bash
ergs importer --port 9090
```

Note the API token printed on startup.

### Step 2: Start Ergs Warehouse

```bash
ergs serve
```

### Step 3: Run the Importer

**Dry run (see what would be imported):**

```bash
./rtve-importer --dry-run --videos-dir ../../rtve-videos
```

**Import all videos:**

```bash
./rtve-importer \
  --videos-dir ../../rtve-videos \
  --api-token "your-api-token" \
  --target-datasource rtve \
  --batch-size 50
```

## Options

- `--videos-dir` - Directory containing RTVE video JSON files (default: `rtve-videos`)
- `--importer-url` - URL of the importer API server (default: `http://localhost:9090`)
- `--api-token` - API token for authentication (required, unless using `--dry-run`)
- `--target-datasource` - Target datasource name in Ergs (default: `rtve`)
- `--batch-size` - Number of blocks to send per request (default: `50`)
- `--dry-run` - Don't actually send blocks, just show what would be imported

## Examples

### Import with custom settings

```bash
./rtve-importer \
  --videos-dir /path/to/rtve-videos \
  --importer-url http://192.168.1.100:9090 \
  --api-token "abc123..." \
  --target-datasource rtve-main \
  --batch-size 100
```

### Dry run to preview

```bash
./rtve-importer --dry-run --videos-dir ../../rtve-videos

# Output:
# RTVE Importer
# =============
# Videos directory: ../../rtve-videos
# Importer URL: http://localhost:9090
# Target datasource: rtve
# Batch size: 50
# Mode: DRY RUN (no blocks will be sent)
#
# Found 150 video JSON files
#
# Would import batch of 50 blocks (total: 50/150)
#   - rtve-16755959: Telediario - 15 horas - 03/10/25
#   - rtve-16746162: Telediario - 21 horas - 26/09/25
#   ...
```

### Import specific directory

```bash
# Import only 2025 videos
./rtve-importer \
  --videos-dir ../../rtve-videos/2025 \
  --api-token "your-token"
```

## How It Works

1. **Scan Directory**: Recursively finds all `video_*.json` files
2. **Parse JSON**: Reads each file and extracts video metadata
3. **Load Subtitles**: 
   - Checks for `subs/` directory next to video JSON file
   - Finds all subtitle languages (e.g., `16755959_es.vtt`, `16755959_en.vtt`)
   - Parses Spanish subtitles (`.vtt` format) or first available language
   - Converts VTT cues to JSON format for storage
4. **Convert to Blocks**: Creates blocks in Ergs format with:
   - ID: `rtve-{videoID}`
   - Type: `rtve`
   - Datasource: Target datasource name (e.g., `rtve`)
   - Metadata: Video details and subtitle text matching RTVE datasource schema
   - Searchable text includes video title AND subtitle content
5. **Batch Import**: Sends blocks in batches (default 50) to the importer API
6. **Authentication**: Uses Bearer token authentication
7. **Progress Tracking**: Shows accepted/rejected counts

## Block Format

Each video is converted to a block with subtitle support:

```json
{
  "id": "rtve-16755959",
  "text": "Telediario - 15 horas - 03/10/25 16755959 es en ca La UCO concluye que Ábalos manejó...",
  "created_at": "2025-10-03T15:00:00Z",
  "type": "rtve",
  "datasource": "rtve",
  "metadata": {
    "video_id": "16755959",
    "long_title": "Telediario - 15 horas - 03/10/25",
    "publication_date": "03-10-2025 15:00:00",
    "html_url": "https://www.rtve.es/play/videos/...",
    "uri": "https://www.rtve.es/api/videos/16755959",
    "has_subtitles": true,
    "subtitle_langs": "es,en,ca,eu,gl",
    "subtitle_text": "[{\"start\":\"00:00:08.537\",\"end\":\"00:00:11.872\",\"text\":\"La UCO concluye que Ábalos manejó, por lo menos,\"}...]"
  }
}
```

**Subtitle Features:**
- All subtitle text is included in the searchable `text` field for full-text search
- `subtitle_text` contains JSON array of VTT cues with timestamps
- `subtitle_langs` lists all available languages (es, en, ca, eu, gl, etc.)
- Spanish subtitles are prioritized for text extraction, falls back to first available

## Verification

After importing, verify blocks are stored:

```bash
# Search for imported videos
ergs search "Telediario"

# Check database
sqlite3 ~/.local/share/ergs/rtve.db \
  "SELECT COUNT(*) FROM blocks WHERE datasource='rtve'"
```

## Troubleshooting

### "Authorization required" error

Make sure you're using the correct API token from the importer server startup logs.

### "Invalid token" error

The token in `--api-token` must match the token configured in the importer server (`[importer] api_token` in config).

### Blocks not appearing in searches

1. Check importer API stats: `curl http://localhost:9090/api/stats`
2. Ensure `ergs serve` is running with importer datasource configured
3. Wait for importer interval (default 5 minutes) or restart `ergs serve`
4. Check that target datasource name matches: `--target-datasource rtve` and `[datasources.rtve]`

### "Connection refused" error

Ensure the importer API server is running on the specified URL/port.

## Customizing for Other Data Sources

This example can be adapted for other data sources:

1. Change the input format (JSON files, CSV, database, API, etc.)
2. Update the parsing logic in `readVideoFile()`
3. Modify `videoToBlock()` to create appropriate metadata
4. Set correct `Type` and `Datasource` fields
5. Ensure target datasource exists in Ergs config

## See Also

- [Importer Datasource Documentation](../../docs/datasources/importer.md)
- [Importer Summary](../../docs/IMPORTER_SUMMARY.md)
- [Complete Example](../../docs/importer-example.md)