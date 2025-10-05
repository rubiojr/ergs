# Chromium Datasource

The Chromium datasource extracts browsing history from Chromium's `History` database file, allowing you to index and search your web browsing activity.

## Overview

This datasource reads visit data from Chromium's local SQLite database, including:
- URLs visited
- Page titles
- Visit timestamps

The datasource safely creates a temporary copy of the database to avoid conflicts with Chromium when it's running.

## Configuration

### Basic Configuration

```toml
[datasources.chromium-browsing]
type = 'chromium'
# interval = '30m0s'  # Optional: custom fetch interval (default: 30m0s)

[datasources.chromium-browsing.config]
database_path = '/home/user/.config/chromium/Default/History'
```

### Required Fields

- `database_path`: Full path to Chromium's `History` file

## Finding Your Chromium Profile

The location of your Chromium profile varies by operating system:

### Linux
```
~/.config/chromium/Default/History
```

For other profiles:
```
~/.config/chromium/Profile 1/History
~/.config/chromium/Profile 2/History
```

### macOS
```
~/Library/Application Support/Chromium/Default/History
```

### Windows
```
%LOCALAPPDATA%\Chromium\User Data\Default\History
```

To find your exact profile path:
1. Open Chromium
2. Navigate to `chrome://version`
3. Look for "Profile Path"
4. The `History` file will be in that directory

## Chromium-based Browsers

This datasource works with any Chromium-based browser that uses the same database schema, including:

- **Google Chrome**: Replace `chromium` with `google-chrome` or `chrome` in the paths
- **Microsoft Edge**: Look in `~/.config/microsoft-edge/` (Linux) or equivalent
- **Brave**: Look in `~/.config/BraveSoftware/Brave-Browser/` (Linux) or equivalent
- **Vivaldi**: Look in `~/.config/vivaldi/` (Linux) or equivalent

### Example: Google Chrome on Linux

```toml
[datasources.chrome]
type = 'chromium'

[datasources.chrome.config]
database_path = '/home/user/.config/google-chrome/Default/History'
```

## Multiple Profiles

You can configure multiple Chromium profiles or different browsers as separate datasources:

```toml
[datasources.chromium-work]
type = 'chromium'

[datasources.chromium-work.config]
database_path = '/home/user/.config/chromium/Profile 1/History'

[datasources.chromium-personal]
type = 'chromium'

[datasources.chromium-personal.config]
database_path = '/home/user/.config/chromium/Default/History'

[datasources.chrome]
type = 'chromium'

[datasources.chrome.config]
database_path = '/home/user/.config/google-chrome/Default/History'
```

## Schema-Only Configuration (For Importer Use)

If you're using the chromium-importer tool to import browsing history remotely, you can configure the datasource with `interval = '0s'` to disable automatic fetching while still providing the schema for storage:

```toml
[datasources.chromium]
type = 'chromium'
interval = '0s'  # Disable automatic fetching (schema-only)
[datasources.chromium.config]
database_path = '/tmp/not-used'  # Dummy path, won't be accessed

[datasources.importer]
type = 'importer'
interval = '5m0s'  # Check for imported blocks every 5 minutes
[datasources.importer.config]
api_url = 'http://localhost:9090'
api_key = 'your-token'
```

This configuration:
- ✅ Registers the chromium schema for storage
- ✅ Accepts imported blocks from chromium-importer
- ✅ No automatic fetching (interval is 0)
- ✅ No error during scheduled fetches
- ✅ Database file is created with proper schema
- ⚠️  May warn at startup if database path doesn't exist (harmless)

See the [chromium-importer documentation](../../importers/chromium-importer/README.md) for details on importing from remote machines.

## Usage

Once configured, the Chromium datasource works with all standard ergs commands:

```bash
# Fetch browsing history
ergs fetch

# Search your browsing history
ergs search --query "documentation"

# List recent visits
ergs list --datasource chromium-browsing --limit 10
```

## Data Fields

Each visit record includes:
- **url**: The visited URL
- **title**: Page title (if available)
- **visit_date**: When the page was visited

## Important Notes

- **Database Locks**: Close Chromium (and all Chromium-based browsers) before running ergs to avoid database locking issues
- **Privacy**: Only visits stored in Chromium's history are included; incognito sessions are not recorded
- **Safety**: The datasource creates a temporary copy of your database file, so your original Chromium data is never modified
- **Performance**: Large browsing histories may take longer to process initially
- **Timestamp Format**: Chromium uses WebKit time format (microseconds since January 1, 1601 UTC), which is automatically converted to standard Unix time

## Troubleshooting

### "Database file does not exist"
- Verify the path to your Chromium profile
- Check that the `History` file exists in the specified location
- Ensure you have read permissions for the file
- Note: The file is named `History` (no extension)

### "Required Chromium tables not found"
- The database file may be corrupted or from an incompatible browser
- Try using a different Chromium profile
- Verify the file is actually a Chromium database (not Firefox or another browser)

### Empty results
- Check that your Chromium profile has browsing history
- Verify Chromium is completely closed when running ergs
- Try running ergs with `--debug` flag for more detailed logging

### Database is locked
- Ensure all Chromium-based browsers are completely closed
- Check that no other processes are accessing the History file
- Wait a few seconds and try again

## Differences from Firefox Datasource

The Chromium datasource is similar to the Firefox datasource but with some key differences:

- **Database Schema**: Uses Chromium's `urls` and `visits` tables instead of Firefox's `moz_places` and `moz_historyvisits`
- **Timestamp Format**: Uses WebKit/Chrome time (microseconds since 1601) instead of Unix microseconds
- **No Description Field**: Chromium doesn't store page descriptions in the history database
- **File Name**: The database is named `History` (not `places.sqlite`)