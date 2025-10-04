# RTVE Datasource

The RTVE datasource fetches TV show episodes from RTVE (Radio Televisión Española), the Spanish public broadcasting corporation. It retrieves episode metadata including titles, publication dates, URLs, and subtitle information.

## Features

- Fetches the latest episodes from RTVE on-demand shows
- Configurable number of episodes to retrieve
- Captures video metadata (title, publication date, URLs)
- Includes subtitle availability and language information
- Supports all available RTVE shows

## Configuration

### Basic Configuration

```toml
[datasources.rtve]
type = 'rtve'
interval = '1h0m0s'  # Fetch every hour

[datasources.rtve.config]
show_id = 'telediario-1'  # Required: RTVE show identifier
max_episodes = 10         # Optional: Maximum episodes to fetch (default: 10)
```

### Configuration Options

| Option | Type | Required | Default | Description |
|--------|------|----------|---------|-------------|
| `show_id` | string | Yes | - | RTVE show identifier (see available shows below) |
| `max_episodes` | integer | No | 10 | Maximum number of latest episodes to fetch (max: 100) |

### Available Shows

The following show IDs are available:

- `telediario-1` - Telediario 1 (main evening news)
- `telediario-2` - Telediario 2 (late night news)
- `telediario-matinal` - Telediario Matinal (morning news)
- `informe-semanal` - Informe Semanal (weekly news magazine)

Use the rtve-go library's `ListShows()` function to get an updated list of available shows.

## Usage Examples

### Fetch Latest Episodes

```bash
# Fetch episodes once
./ergs fetch

# Stream episodes to stdout
./ergs fetch --stream
```

### Search Episodes

```bash
# Search for specific topics in episodes
./ergs search --query "economía"

# Search by date range
./ergs search --query "política" --after "2024-01-01"

# Filter by source
./ergs search --query "internacional" --source "rtve"
```

### Multiple Shows

You can configure multiple RTVE datasources for different shows:

```toml
[datasources.telediario1]
type = 'rtve'
interval = '1h0m0s'
[datasources.telediario1.config]
show_id = 'telediario-1'
max_episodes = 20

[datasources.informesemanal]
type = 'rtve'
interval = '6h0m0s'  # Less frequent for weekly show
[datasources.informesemanal.config]
show_id = 'informe-semanal'
max_episodes = 5
```

## Block Structure

Each RTVE episode is stored as a block with the following metadata:

- `video_id` - Unique RTVE video identifier
- `long_title` - Full episode title
- `publication_date` - Publication date in RTVE format (DD-MM-YYYY HH:MM:SS)
- `html_url` - URL to watch the video on RTVE website
- `uri` - RTVE API URI
- `has_subtitles` - Whether subtitles are available (boolean)
- `subtitle_langs` - Comma-separated list of available subtitle languages

### Example Block Output

```
📺 Telediario 1 - 15/01/2024
  ID: 7234567
  Published: 2024-01-15 21:00:00
  Subtitles: Yes (es, ca)
  URL: https://www.rtve.es/play/videos/telediario/...
```

## Implementation Details

### How It Works

1. The datasource uses the [rtve-go](https://github.com/rubiojr/rtve-go) library (v0.2.0)
2. Fetches the latest N episodes using `api.FetchShowLatest()`
3. For each episode:
   - Extracts video metadata (title, ID, publication date, URLs)
   - Checks for subtitle availability and languages
   - Creates a searchable block with all metadata
4. Blocks are indexed for full-text search

### Searchable Content

The following fields are indexed for full-text search:
- Episode title
- Video ID
- Subtitle languages

### Rate Limiting

RTVE's API is accessed through web scraping, so:
- The rtve-go library includes appropriate delays
- Use reasonable `max_episodes` values to avoid excessive requests
- Set longer intervals for shows that update infrequently

## Troubleshooting

### No Episodes Found

If no episodes are fetched:

1. Verify the `show_id` is correct and still available
2. Check your internet connection
3. Ensure RTVE's website is accessible
4. Check the logs for specific error messages

### Invalid Show ID Error

```
Error: invalid show_id: xyz (available shows: [...])
```

This means the configured `show_id` is not recognized. Use one of the available show IDs listed in the error message.

### Subtitle Fetch Errors

Non-fatal errors when fetching subtitles are logged but don't stop episode fetching. The episode will still be saved with `has_subtitles: false`.

## Dependencies

This datasource requires:
- [rtve-go](https://github.com/rubiojr/rtve-go) v0.2.0

The dependency is automatically managed through Go modules.

## Privacy Notes

- This datasource only fetches publicly available RTVE content
- No authentication or personal data is required
- Episode metadata and subtitles are stored locally in your Ergs database
- Subtitle text (if available) is included in the searchable content

## Related

- [RTVE-Go Library](https://github.com/rubiojr/rtve-go) - The underlying library used by this datasource
- [RTVE Play](https://www.rtve.es/play/) - RTVE's on-demand streaming platform