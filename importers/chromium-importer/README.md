# Chromium Importer

This is a standalone importer that reads browsing history from a Chromium database and imports it into Ergs via the importer API.

## Overview

This tool allows you to import Chromium browsing history from remote machines or local databases into a central Ergs instance. It's particularly useful for:

1. **Remote Data Collection**: Import browsing history from multiple machines to a central Ergs server
2. **Chromium Running**: Import history even while Chromium is running (creates a temporary copy)
3. **Historical Imports**: Import large amounts of browsing history in batches
4. **Cross-Machine Sync**: Consolidate browsing history from multiple computers

## Architecture

```
Chromium Database (History)
    ↓
Chromium Importer (this tool)
    ↓ POST /api/import/blocks
Importer API Server (ergs importer)
    ↓
Staging Database (importer.db)
    ↓
Importer Datasource (ergs serve)
    ↓
Warehouse
    ↓
chromium.db (native storage with type="chromium")
```

## Prerequisites

### 1. Ergs Server Setup

Configure Ergs with both chromium and importer datasources:

```toml
# ~/.config/ergs/config.toml

[importer]
api_key = "your-api-token-here"

[datasources.chromium]
type = 'chromium'
interval = '0s'  # Disable automatic fetching (schema-only, for importer use)
[datasources.chromium.config]
database_path = '/tmp/chromium-not-used'  # Dummy path (won't be fetched)

[datasources.importer]
type = 'importer'
interval = '5m0s'
[datasources.importer.config]
api_url = 'http://localhost:9090'
api_key = 'your-api-token-here'
```

### 2. Start Ergs Services

```bash
# Terminal 1: Start the importer API server
ergs importer --port 9090

# Note the API token from the output, or configure one in config.toml

# Terminal 2: Start the warehouse
ergs serve
```

## Finding Your Chromium Database

The Chromium History database location varies by browser and OS:

### Chromium (Linux)
```
~/.config/chromium/Default/History
~/.config/chromium/Profile 1/History
```

### Google Chrome (Linux)
```
~/.config/google-chrome/Default/History
~/.config/google-chrome/Profile 1/History
```

### Chromium (macOS)
```
~/Library/Application Support/Chromium/Default/History
```

### Google Chrome (macOS)
```
~/Library/Application Support/Google/Chrome/Default/History
```

### Chromium (Windows)
```
%LOCALAPPDATA%\Chromium\User Data\Default\History
```

### Google Chrome (Windows)
```
%LOCALAPPDATA%\Google\Chrome\User Data\Default\History
```

### Other Chromium-based Browsers

- **Brave**: `~/.config/BraveSoftware/Brave-Browser/Default/History`
- **Microsoft Edge**: `~/.config/microsoft-edge/Default/History`
- **Vivaldi**: `~/.config/vivaldi/Default/History`

To find your exact path, navigate to `chrome://version` in your browser and look for "Profile Path".

## Building

```bash
cd importers/chromium-importer
go build -o chromium-importer
```

## Usage

### Basic Import

```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --api-token "your-api-token" \
  --target-datasource chromium
```

### Dry Run (Preview)

See what would be imported without actually sending data:

```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --dry-run
```

Example output:
```
Chromium Importer
================
Database path: /home/user/.config/chromium/Default/History
Importer URL: http://localhost:9090
Target datasource: chromium
Batch size: 100
Mode: DRY RUN (no blocks will be sent)

Found 15234 visits in database

Would import batch of 100 blocks (total: 100)
  - chromium-visit-6344: https://chromium.googlesource.com/...
  - chromium-visit-6343: https://github.com/...
  - chromium-visit-6342: https://example.com/...
  - chromium-visit-6341: https://stackoverflow.com/...
  - chromium-visit-6340: https://news.ycombinator.com/...
  ... and 95 more
```

### Import with Limit

Import only the most recent 1000 visits:

```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --api-token "your-api-token" \
  --limit 1000
```

### Remote Import

Import from a remote Ergs server:

```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --importer-url http://192.168.1.100:9090 \
  --api-token "your-api-token" \
  --target-datasource chromium-laptop
```

### Custom Batch Size

Adjust batch size for network conditions:

```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --api-token "your-api-token" \
  --batch-size 50  # Smaller batches for slower networks
```

## Command-Line Options

| Option | Default | Description |
|--------|---------|-------------|
| `--database-path` | (required) | Path to Chromium History database |
| `--importer-url` | `http://localhost:9090` | URL of the importer API server |
| `--api-token` | (required*) | API token for authentication |
| `--target-datasource` | `chromium` | Target datasource name in Ergs |
| `--batch-size` | `100` | Number of blocks to send per request |
| `--limit` | `0` (all) | Maximum number of visits to import |
| `--dry-run` | `false` | Preview mode, don't send blocks |

*Not required when using `--dry-run`

## How It Works

1. **Database Copy**: Creates a temporary copy of the Chromium database to avoid locking issues
2. **Read History**: Queries the `urls` and `visits` tables (ordered by most recent)
3. **Convert Timestamps**: Converts Chrome time (microseconds since 1601-01-01) to Unix time
4. **Create Blocks**: Converts each visit into an Ergs block with:
   - ID: `chromium-visit-{visit_id}`
   - Type: `chromium`
   - Datasource: Target datasource name (e.g., `chromium`)
   - Metadata: URL, title, visit date
   - Searchable text: URL and title
5. **Batch Upload**: Sends blocks in batches to the importer API
6. **Cleanup**: Removes temporary database copy

## Block Format

Each visit is converted to a block:

```json
{
  "id": "chromium-visit-6344",
  "text": "url=https://example.com title=Example Domain",
  "created_at": "2025-10-05T13:46:46Z",
  "type": "chromium",
  "datasource": "chromium",
  "metadata": {
    "url": "https://example.com",
    "title": "Example Domain",
    "visit_date": "2025-10-05 13:46:46",
    "source": "chromium"
  }
}
```

## Use Cases

### Schema-Only Datasource Configuration

When using ONLY the chromium-importer (not direct database fetching), configure the datasource with `interval = '0s'` to disable automatic fetching:

```toml
[datasources.chromium]
type = 'chromium'
interval = '0s'  # Disable automatic fetching
[datasources.chromium.config]
database_path = '/tmp/not-used'  # Dummy path, won't be accessed

[datasources.importer]
type = 'importer'
interval = '5m0s'
[datasources.importer.config]
api_url = 'http://localhost:9090'
api_key = 'your-token'
```

This configuration:
- ✅ Registers the chromium schema for storage
- ✅ Accepts imported blocks from chromium-importer
- ✅ No automatic fetching (interval is 0)
- ✅ No warnings about missing database
- ✅ Warehouse only fetches from importer datasource

### Import from Multiple Machines

Set up the importer to run on multiple machines and import to a central server:

**Machine 1 (laptop):**
```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --importer-url http://central-server:9090 \
  --api-token "shared-token" \
  --target-datasource chromium-laptop
```

**Machine 2 (desktop):**
```bash
./chromium-importer \
  --database-path ~/.config/google-chrome/Default/History \
  --importer-url http://central-server:9090 \
  --api-token "shared-token" \
  --target-datasource chromium-desktop
```

**Machine 3 (work):**
```bash
./chromium-importer \
  --database-path ~/.config/chromium/Profile\ 1/History \
  --importer-url http://central-server:9090 \
  --api-token "shared-token" \
  --target-datasource chromium-work
```

### Scheduled Imports

Use cron to periodically import new history:

```bash
# crontab -e
# Import every 6 hours
0 */6 * * * /path/to/chromium-importer --database-path ~/.config/chromium/Default/History --api-token "your-token" --limit 1000
```

### Migrate Historical Data

Import all browsing history when setting up Ergs:

```bash
./chromium-importer \
  --database-path ~/.config/chromium/Default/History \
  --api-token "your-token"
  # No --limit flag = import everything
```

## Verification

After importing, verify blocks are stored:

```bash
# Search for imported visits
ergs search "example.com"

# Check database
sqlite3 ~/.local/share/ergs/chromium.db \
  "SELECT COUNT(*) FROM blocks WHERE datasource='chromium'"

# List recent imports
ergs list --datasource chromium --limit 10
```

## Troubleshooting

### "Database file does not exist"

- Verify the path to your Chromium database
- Check that you have read permissions
- Make sure you're using the correct browser's database path

### "Required Chromium tables not found"

- The database file may be from a different browser (Firefox, Safari, etc.)
- Verify it's actually a Chromium/Chrome database
- Check the database isn't corrupted

### "Authorization required" or "Invalid token"

- Ensure you're using the correct API token
- The token must match what's configured in the importer server
- Check the importer server startup logs for the token

### "Connection refused"

- Ensure the importer API server is running (`ergs importer`)
- Verify the URL and port are correct
- Check firewall settings if connecting remotely

### Import is very slow

- Reduce `--batch-size` to send smaller batches
- Use `--limit` to import fewer visits
- Check network connectivity if importing remotely

### Duplicate visits

- Each visit has a unique ID (`chromium-visit-{id}`)
- Re-running the importer will skip duplicates automatically
- The warehouse handles deduplication by block ID

## Important Notes

- **Database Locks**: The importer creates a temporary copy, so Chromium can remain running
- **Privacy**: Only visits in Chromium's history are imported (incognito sessions are not recorded)
- **Safety**: Your original Chromium database is never modified
- **Bandwidth**: Large imports can take time; use `--limit` for testing
- **Deduplication**: Re-running the importer is safe; duplicate blocks are ignored

## See Also

- [Chromium Datasource Documentation](../../docs/datasources/chromium.md)
- [Importer Datasource Documentation](../../docs/datasources/importer.md)
- [RTVE Importer Example](../rtve-importer/README.md)