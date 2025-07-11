# Firefox Datasource

The Firefox datasource extracts browsing history from Firefox's `places.sqlite` database file, allowing you to index and search your web browsing activity.

## Overview

This datasource reads visit data from Firefox's local SQLite database, including:
- URLs visited
- Page titles
- Descriptions (when available)
- Visit timestamps

The datasource safely creates a temporary copy of the database to avoid conflicts with Firefox when it's running.

## Configuration

### Basic Configuration

```toml
[datasources.firefox-browsing]
type = 'firefox'

[datasources.firefox-browsing.config]
database_path = '/path/to/your/firefox/profile/places.sqlite'
```

### Required Fields

- `database_path`: Full path to Firefox's `places.sqlite` file

## Finding Your Firefox Profile

The location of your Firefox profile varies by operating system:

### Linux
```
~/.mozilla/firefox/PROFILE_NAME/places.sqlite
```

### macOS
```
~/Library/Application Support/Firefox/Profiles/PROFILE_NAME/places.sqlite
```

### Windows
```
%APPDATA%\Mozilla\Firefox\Profiles\PROFILE_NAME\places.sqlite
```

To find your exact profile path:
1. Open Firefox
2. Navigate to `about:profiles`
3. Look for "Root Directory" under your active profile
4. The `places.sqlite` file will be in that directory

## Multiple Profiles

You can configure multiple Firefox profiles as separate datasources:

```toml
[datasources.firefox-work]
type = 'firefox'

[datasources.firefox-work.config]
database_path = '/home/user/.mozilla/firefox/work.profile/places.sqlite'

[datasources.firefox-personal]
type = 'firefox'

[datasources.firefox-personal.config]
database_path = '/home/user/.mozilla/firefox/personal.profile/places.sqlite'
```

## Usage

Once configured, the Firefox datasource works with all standard ergs commands:

```bash
# Fetch browsing history
ergs fetch

# Search your browsing history
ergs search --query "github"

# List recent visits
ergs list --datasource firefox-browsing --limit 10
```

## Data Fields

Each visit record includes:
- **url**: The visited URL
- **title**: Page title (if available)
- **description**: Page description/meta description (if available)
- **visit_date**: When the page was visited

## Important Notes

- **Database Locks**: Close Firefox before running ergs to avoid database locking issues
- **Privacy**: Only visits stored in Firefox's history are included; private browsing sessions are not recorded
- **Safety**: The datasource creates a temporary copy of your database file, so your original Firefox data is never modified
- **Performance**: Large browsing histories may take longer to process initially

## Troubleshooting

### "Database file does not exist"
- Verify the path to your Firefox profile
- Check that the `places.sqlite` file exists in the specified location
- Ensure you have read permissions for the file

### "Required Firefox tables not found"
- The database file may be corrupted or from a very old Firefox version
- Try using a different Firefox profile
- Verify the file is actually a Firefox database (not Chrome or another browser)

### Empty results
- Check that your Firefox profile has browsing history
- Verify Firefox is completely closed when running ergs
- Try running ergs with `--debug` flag for more detailed logging