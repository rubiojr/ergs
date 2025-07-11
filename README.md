# Ergs - Datahoarder's Paradise

A flexible data fetching and indexing tool that collects information from various sources and makes it searchable. Perfect for digital packrats who want to hoard and search their data.

## Quick Start

### 1. Build & Install
```bash
git clone https://github.com/your-username/ergs
cd ergs
make build
```

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

### 6. Run Continuously
```bash
# Fetches new data every 30 minutes by default
./ergs serve
```

## Available Datasources

- **GitHub** - Your GitHub activity, starred repos, and interactions
- **HackerNews** - Stories, comments, jobs, and polls from Hacker News
- **Firefox** - Complete browsing history with full-text search
- **Codeberg** - Codeberg activity and repository events
- **Zed Threads** - Chat history from Zed editor
- **Gas Stations** - Local gas station prices and info
- **Timestamp** - Simple timestamp logging (useful for testing)

## Common Commands

```bash
# See what datasources you have
./ergs datasource list

# Check your data stats
./ergs stats

# List recent items from a datasource
./ergs list --datasource github --limit 5
```

## Configuration

Your config lives at `~/.config/ergs/config.toml`. You can edit it directly or use the CLI commands. Each datasource gets its own SQLite database with full-text search.

## Need Help?

- Check the [datasource documentation](docs/datasources/) for detailed setup instructions
- See [docs/queries.md](docs/queries.md) for FTS5 search syntax and examples
- See [docs/datasource.md](docs/datasource.md) if you want to create your own datasources
- Run `./ergs --help` for all available commands

Start hoarding! 📦
