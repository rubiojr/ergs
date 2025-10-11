![Ergs - Datahoarder's Paradise](/images/dune-logo-banner.svg)

A flexible data fetching and indexing tool that collects information from various sources and makes it searchable. Perfect for digital packrats who want to hoard and search their data.

> [!INFO]
> Ergs is currently Beta quality, public facing APIs are quickly evolving
> and currently in flux

## Quick Start

### 1. Build & Install

Use [Docker](/docker/DOCKER.md) or build from source:

```bash
git clone https://github.com/rubiojr/ergs
cd ergs
make build
```

Or grab a binary release from https://github.com/rubiojr/ergs/releases.

### 2. Initialize
```bash
./ergs init
```

### 4. Fetch Data
```bash
./ergs fetch
```

### 5. Search Everything
```bash
# Search all your data
./ergs search --query "language typescript"

# Search specific datasource
./ergs search --datasource github --query "rust"
```

### 6. Web Interface
```bash
# Start web interface with search and browsing
./ergs web --port 8080

# Then visit http://localhost:8080
```

### 7. Run Continuously
```bash
# Fetches new data every 30 minutes by default
./ergs serve
```

## Available Datasources

### Browser Data
- **Firefox** - Complete browsing history with full-text search from Firefox's places.sqlite database
- **Chromium** - Browsing history from Chromium-based browsers (Chrome, Edge, Brave, etc.)

### Code Hosting Platforms
- **GitHub** - Your GitHub activity, starred repos, and interactions
- **Codeberg** - Codeberg activity and repository events

### News & Media
- **HackerNews** - Stories, comments, jobs, and polls from Hacker News
- **RSS** - Articles from RSS/Atom feeds (blogs, news sites, etc.)
- **RTVE** - TV show episodes from RTVE (Spanish public broadcasting)

### Development Tools
- **Zed Threads** - AI conversation threads from Zed editor

### External Data Import
- **Importer** - HTTP API for importing blocks from external sources and custom scripts

### Utilities
- **Gas Stations** - Local gas station prices and info
- **Datadis** - Electricity consumption data from Datadis (Spanish electricity data platform)
- **Timestamp** - Simple timestamp logging (useful for testing)

## Common Commands

```bash
# See what datasources you have
./ergs datasource list

# Check your data stats
./ergs stats

# List recent items from a datasource
./ergs list --datasource github --limit 5

# Start web interface for browsing and search
./ergs web
```

## Configuration

Your config lives at `~/.config/ergs/config.toml`. You can edit it directly or use the CLI commands. Each datasource gets its own SQLite database with full-text search.

## Need Help?

- Check the [datasource documentation](docs/datasources/) for detailed setup instructions
- See [docs/web-interface.md](docs/web-interface.md) for web interface and API documentation
- See [docs/queries.md](docs/queries.md) for FTS5 search syntax and examples
- See [docs/datasource.md](docs/datasource.md) if you want to create your own datasources
- Run `./ergs --help` for all available commands

Start hoarding! 📦
