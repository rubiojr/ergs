# Chromium Datasource

This package implements a datasource for extracting browsing history from Chromium-based browsers.

## Overview

The Chromium datasource reads from the `History` SQLite database used by Chromium and Chromium-based browsers (Chrome, Edge, Brave, Vivaldi, etc.). It extracts:

- URLs visited
- Page titles
- Visit timestamps

## Database Schema

The datasource queries the following Chromium tables:

- `urls`: Contains URL information and titles
- `visits`: Contains visit timestamps and metadata

## Timestamp Conversion

Chromium uses WebKit/Chrome time format (microseconds since January 1, 1601 UTC). The datasource automatically converts these timestamps to standard Unix time using the `chromeTimeToUnix()` function.

The conversion formula:
```
unix_seconds = (chrome_time_microseconds / 1000000) - 11644473600
```

Where 11644473600 is the number of seconds between the Chrome epoch (1601-01-01) and Unix epoch (1970-01-01).

## Supported Browsers

This datasource works with any Chromium-based browser that uses the standard History database schema:

- Chromium
- Google Chrome
- Microsoft Edge
- Brave Browser
- Vivaldi
- Opera (newer versions)

## Implementation Details

### Safety Features

1. **Read-Only Access**: The datasource opens databases in read-only mode
2. **Temporary Copy**: Creates a temporary copy of the database to avoid locking issues
3. **Database Validation**: Checks for required tables before attempting to read data

### Block Structure

Each visit creates a `VisitBlock` containing:
- **ID**: `chromium-visit-{visit_id}`
- **URL**: The visited URL
- **Title**: Page title from the database
- **Visit Date**: Timestamp of the visit
- **Source**: The datasource instance name

### Error Handling

The datasource handles common issues:
- Missing database files
- Locked databases (when browser is running)
- Missing required tables
- Database read errors

## Testing

Run the tests with:

```bash
make test
```

Or specifically for this package:

```bash
go test ./pkg/datasources/chromium/... -v --tags fts5
```

## Configuration

See the [main documentation](../../../docs/datasources/chromium.md) for configuration examples and usage instructions.