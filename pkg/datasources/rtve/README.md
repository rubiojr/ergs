# RTVE Datasource

This package implements a datasource for fetching TV show episodes from RTVE (Radio Televisión Española), the Spanish public broadcasting corporation.

## Overview

The RTVE datasource fetches the latest episodes from RTVE on-demand shows using the [rtve-go](https://github.com/rubiojr/rtve-go) library. It retrieves episode metadata including titles, publication dates, URLs, and subtitle information.

## Features

- Fetches latest episodes for configured RTVE shows
- Configurable maximum number of episodes to retrieve
- Captures video metadata (title, publication date, URLs)
- Includes subtitle availability and language information
- Self-registration via init() function
- Full-text searchable content

## Configuration

```toml
[datasources.rtve]
type = 'rtve'
interval = '1h0m0s'

[datasources.rtve.config]
show_id = 'telediario-1'  # Required: RTVE show identifier
max_episodes = 10         # Optional: Maximum episodes to fetch (default: 10, max: 100)
```

### Available Shows

Common show IDs:
- `telediario-1` - Telediario 1 (main evening news)
- `telediario-2` - Telediario 2 (late night news)
- `telediario-matinal` - Telediario Matinal (morning news)
- `informe-semanal` - Informe Semanal (weekly news magazine)

Use `api.AvailableShows()` from rtve-go to get the complete list.

## Block Structure

Each episode is stored as an `RTVEBlock` with the following metadata:

- `video_id` - Unique RTVE video identifier
- `long_title` - Full episode title
- `publication_date` - Publication date in RTVE format (DD-MM-YYYY HH:MM:SS)
- `html_url` - URL to watch the video
- `uri` - RTVE API URI
- `has_subtitles` - Whether subtitles are available
- `subtitle_langs` - Comma-separated list of subtitle languages

## Implementation Details

### Architecture

The datasource follows the standard Ergs datasource pattern:

1. **Configuration** (`Config` struct) - Validates show ID and episode limits
2. **Datasource** (`Datasource` struct) - Implements `core.Datasource` interface
3. **Block** (`RTVEBlock` struct) - Implements `core.Block` interface
4. **Self-registration** - Automatic registration via `init()` function

### Streaming Pattern

The datasource uses the rtve-go library's `FetchShowLatest()` function with a visitor pattern:

```go
visitor := func(result *api.VideoResult) error {
    // Create block from video metadata
    block := NewRTVEBlockWithSource(...)
    
    // Stream through channel
    blockCh <- block
    return nil
}

api.FetchShowLatest(showID, maxEpisodes, visitor)
```

This allows real-time streaming of episodes as they're fetched from RTVE.

### Data Isolation

The datasource properly uses instance names (not datasource types) as the source identifier, ensuring proper data isolation when multiple RTVE datasources are configured for different shows.

## Usage Examples

### Single Show

```toml
[datasources.telediario]
type = 'rtve'
interval = '2h0m0s'
[datasources.telediario.config]
show_id = 'telediario-1'
max_episodes = 20
```

### Multiple Shows

```toml
[datasources.telediario1]
type = 'rtve'
interval = '1h0m0s'
[datasources.telediario1.config]
show_id = 'telediario-1'
max_episodes = 15

[datasources.informesemanal]
type = 'rtve'
interval = '6h0m0s'
[datasources.informesemanal.config]
show_id = 'informe-semanal'
max_episodes = 5
```

## Web Rendering

The package includes a web renderer (`cmd/web/renderers/rtve`) that displays RTVE episodes with:

- RTVE branding and colors
- Episode title and metadata
- Publication date
- Subtitle availability
- Direct link to watch on RTVE website
- Responsive design
- Dark mode support

## Dependencies

- [rtve-go](https://github.com/rubiojr/rtve-go) v0.2.0 - RTVE API client library

## Error Handling

- Invalid show IDs are validated at configuration time
- Network errors are propagated to the fetch operation
- Subtitle fetch errors are logged but don't stop episode fetching
- Context cancellation is respected throughout the fetch process

## Testing

The datasource is tested through the standard Ergs test suite. Integration tests verify:

- Configuration validation
- Block creation and metadata
- Database storage and retrieval
- Factory pattern reconstruction

## Future Enhancements

Potential improvements:

- Date range filtering for episode fetching
- Support for fetching specific episode IDs
- Caching of already-fetched episodes to avoid duplicates
- Integration with subtitle download functionality
- Episode description text extraction