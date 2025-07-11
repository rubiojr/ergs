# Datasources

This directory contains documentation for individual datasources available in Ergs. Each datasource has its own configuration options and behavior.

## Available Datasources

### Browser Data
- **[Firefox](firefox.md)** - Extract browsing history from Firefox's places.sqlite database

### Code Hosting Platforms
- **[GitHub](github.md)** - Fetch GitHub activity, events, and repository interactions
- **[Codeberg](codeberg.md)** - Fetch Codeberg activity and repository events

### Development Tools
- **[Zed Threads](zedthreads.md)** - Extract AI conversation threads from Zed editor

## General Usage

All datasources follow the same configuration pattern in your `config.toml`:

```toml
[datasources.my-datasource-name]
type = 'datasource_type'

[datasources.my-datasource-name.config]
# datasource-specific configuration options
```

## Common Commands

Once configured, all datasources work with the same ergs commands:

```bash
# List configured datasources
ergs datasource list

# Fetch data from all datasources
ergs fetch

# Fetch from specific datasource
ergs fetch --datasource my-datasource-name

# Search across all data
ergs search --query "search terms"

# List recent data from a datasource
ergs list --datasource my-datasource-name --limit 10

# View statistics
ergs stats
```

## Development

For information on creating new datasources, see the main [Datasource Development Guide](../datasource.md).