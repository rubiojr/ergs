# Ergs Documentation

Welcome to the Ergs documentation! Ergs is a flexible, extensible data fetching and indexing system that can collect blocks of data from various internet APIs and provide full-text search capabilities.

## Getting Started

- **[Quick Start Guide](../README.md#quick-start)** - Get up and running with Ergs in minutes
- **[Installation](../README.md#installation)** - Build and install Ergs
- **[Configuration](../README.md#configuration)** - Basic configuration setup

## Architecture

- **[Architecture Overview](architecture.md)** - System design and core concepts
- **[System Components](../README.md#architecture-overview)** - Blocks, Datasources, Warehouse, and Registration

## Datasources

- **[Available Datasources](datasources/)** - Complete list of supported datasources with configuration examples
- **[Firefox](datasources/firefox.md)** - Extract browsing history from Firefox
- **[GitHub](datasources/github.md)** - Fetch GitHub activity and events
- **[Codeberg](datasources/codeberg.md)** - Fetch Codeberg activity and events
- **[Zed Threads](datasources/zedthreads.md)** - Extract AI conversation threads from Zed editor

## Development

- **[Datasource Development Guide](datasource.md)** - Complete guide for creating new datasources
- **[Contributing](../README.md#development)** - Development setup and project structure

## Usage

### Basic Commands

```bash
# Initialize configuration
ergs init

# Add datasources
ergs datasource add --name my-github --type github --token "your-token"

# Fetch data
ergs fetch

# Search data
ergs search --query "your search terms"

# Start daemon
ergs serve --interval 30m
```

### Common Workflows

1. **First Time Setup**
   - Initialize config with `ergs init`
   - Add datasources for your platforms
   - Run initial fetch with `ergs fetch`

2. **Daily Usage**
   - Search your data with `ergs search`
   - View recent activity with `ergs list`
   - Check statistics with `ergs stats`

3. **Automated Collection**
   - Run daemon with `ergs serve`
   - Configure fetch intervals
   - Monitor logs for issues

## Configuration Examples

### Basic Configuration
```toml
storage_dir = '/home/user/.local/share/ergs'
fetch_interval = '30m0s'

[datasources.github-activity]
type = 'github'

[datasources.github-activity.config]
token = 'your-github-token'
```

### Multi-Source Configuration
```toml
[datasources.firefox-browsing]
type = 'firefox'

[datasources.firefox-browsing.config]
database_path = '/home/user/.mozilla/firefox/profile/places.sqlite'

[datasources.github-work]
type = 'github'

[datasources.github-work.config]
token = 'work-token'
language = 'Go'

[datasources.codeberg-personal]
type = 'codeberg'

[datasources.codeberg-personal.config]
username = 'myusername'
token = 'personal-token'

[datasources.zed-threads]
type = 'zedthreads'

# Uses default path: ~/.local/share/zed/threads/threads.db
```

## Troubleshooting

### Common Issues

- **Database locked errors**: Make sure source applications (like Firefox) are closed
- **Rate limit errors**: Add API tokens for higher rate limits
- **No results found**: Check datasource configuration and permissions
- **Build errors**: Ensure you're using `--tags fts5` for SQLite FTS5 support

### Debug Mode

Enable debug logging for troubleshooting:
```bash
ergs --debug fetch
```

### Getting Help

- Check individual datasource documentation for specific issues
- Review error messages carefully - they usually indicate the specific problem
- Use debug mode to get more detailed logging
- Check that file paths and tokens are correct in your configuration

## File Locations

- **Configuration**: `~/.config/ergs/config.toml`
- **Data Storage**: `~/.local/share/ergs/` (or configured `storage_dir`)
- **Logs**: Console output (use `--debug` for verbose logging)

## Project Structure

```
ergs/
├── docs/                    # Documentation
│   ├── datasources/        # Datasource-specific docs
│   ├── architecture.md     # System architecture
│   └── datasource.md       # Development guide
├── pkg/                     # Go packages
│   ├── core/               # Core interfaces
│   ├── datasources/        # Datasource implementations
│   ├── storage/            # Storage layer
│   └── warehouse/          # Data warehouse
├── main.go                 # Application entry point
├── datasources.go          # Datasource registration
└── README.md               # Project overview
```
