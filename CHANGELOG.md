# Changelog

## [NEXT] - TBD

- Drop unknown/unconfigured datasource blocks to prevent implicit DB creation

## [3.6.0] - 2025-10-11

> [!WARNING]
> This version requires running the `migrate` command before starting Ergs services.

### ✨ New Features

- datadis: new datasource (https://datadis.es) and importer

### 🔧 Improvements

- homeassistant: the datasource is more reliable under rough network conditions
- services, cli: don't error when unused datasource databases are around
- docker: realtime firehose is enabled by default when using Docker compose
- block ingestion time is now stored

## [3.5.1] - 2025-10-10

### 🔧 Improvements

- Home Assistant datasource fixes

## [3.5.0] - 2025-10-10

### ✨ New Features

- **Realtime firehose**: Watch blocks being stored realtime.
- **New Home Assistant datastore**: Pulls events from Home Assistant websocket API.

## [3.4.2] - 2025-10-09

### 🔧 Improvements

- Fix timestamps in some datasources

## [3.4.1] - 2025-10-09

### 🔧 Improvements

- Fix firehose block ordering issues

## [3.4.0] - 2025-10-09

### 🔧 Improvements

> [!WARNING]
> This version requires you run `ergs migrate` manually before starting Ergs services if you are upgrading and not using docker compose.

- **Database layer improvements**: fix FTS5 index consistency issues.

## [3.3.0] - 2025-10-08

### ✨ New Features

- **New Style**: Dark and light themes for Ergs web using the Nord color palettes

## [3.2.0] - 2025-10-08

### ✨ New Features

- **Firehose page**: Ergs web page to list latest blocks stored

## [3.1.4] - 2025-10-08

### 🔧 Improvements

- Web UI layout tweaks
- Exclude importer from the datasources list

## [3.1.3] - 2025-10-08

### 🔧 Improvements

- Fixed multiple instance support in github, codeberg and rss datasources

## [3.1.2] - 2025-10-07

### 🔧 Improvements

- Fixed API bug where date filtering failed when used without a search query.

## [3.1.1] - 2025-10-05

- Docker images published

## [3.1.0] - 2025-10-05

### ✨ New Features

- **Chromium Datasource**: New datasource for extracting browsing history from Chromium-based browsers
  - Extracts URLs, page titles, and visit timestamps
  - Full documentation in `docs/datasources/chromium.md`

- **Chromium Importer**: External importer tool for importing browsing history from remote machines
  - Located in `importers/chromium-importer/`
  - Import history via HTTP to central Ergs instance
  - Supports batch importing with configurable batch size
  - Can import from multiple machines to single Ergs server
  - Includes systemd service and timer units for automated imports

- **Schema-Only Datasources (Interval 0)**: New capability to disable automatic fetching while preserving schema
  - Set `interval = '0s'` to register datasource schema without automatic fetching
  - Useful for datasources that only receive data via importer API

---

## [3.0.1] - 2025-10-04

### 🔧 Improvements

- **Importer Configuration**: Host and port can now be configured in `config.toml`
  - Falls back to command-line flags if not set in config
  - Default: `localhost:9090`

---

## [3.0.0] - 2025-10-04

### ✨ New Features

- **Block Import/Export System**: New importer API and datasource for pushing blocks from external sources
  - `ergs importer` - HTTP API server for receiving blocks from external importers
  - Importer datasource that routes incoming blocks to target datasources
  - API key authentication for secure imports
  - Staging database prevents data loss if target datasource fails
  - Example RTVE importer showing how to build external importers
  - Full documentation in `docs/datasources/importer.md`

- **Database Optimization Commands**: Complete suite of database maintenance tools
  - `ergs optimize check` - Deep integrity checks including FTS5-specific corruption detection
    - Detects FTS/blocks table sync issues that standard checks miss
    - Tests actual queries to find corruption that only appears at query time
    - `--quick` flag to skip deep FTS checks for faster operation
  - `ergs optimize fts-rebuild` - Smart FTS5 index rebuilding
    - Automatically checks integrity first and only rebuilds if needed
    - `--force` flag to skip checks and rebuild unconditionally
    - Shows which databases were rebuilt vs skipped
  - `ergs optimize analyze` - Update query planner statistics
  - `ergs optimize vacuum` - Defragment and reclaim disk space
  - `ergs optimize checkpoint` - WAL checkpoint to flush writes
  - `ergs optimize all` - Run all optimization operations
  - All commands support `--datasource <name>` to target specific databases
  - Real-time progress reporting with ✓/✗ indicators

### 🔧 Improvements

- **Enhanced FTS Corruption Detection**: New `FTSIntegrityCheck` method catches corruption missed by standard SQLite integrity checks
  - Tests multiple query patterns (simple MATCH, phrase queries, multi-word phrases)
  - Retrieves actual content from blocks table to detect missing rows
  - Critical for external content tables where FTS index can reference deleted rows

- **Progress Reporting**: All optimize commands show real-time progress as each database is processed

### 📚 Documentation

- Added comprehensive importer documentation with architecture diagrams
- Added example external importer implementation (rtve-importer)

---

## [2.2.1] - 2025-10-04

### ✨ New Features

- **RTVE Transcript Copy**: Added clipboard copy button to RTVE subtitle transcripts
  - One-click copy of full transcript with timestamps
  - Visual feedback when copied successfully
  - Formatted as `[timestamp] text` for easy reading

---

## [2.2.0] - 2025-10-04

### ✨ New Features

- **RTVE Subtitle Support**: Enhanced RTVE datasource with Spanish subtitle parsing
  - Downloads and parses Spanish VTT subtitles for full-text search
  - Subtitles stored as structured JSON (timestamps + text)
  - Beautiful collapsible transcript view in web interface with timestamps
  - Makes episode content fully searchable

---

## [2.1.2] - 2025-10-04

- Bump deps to fix rtve datasource issues

---

## [2.1.1] - 2025-10-04

### ✨ New Features

- **RTVE Datasource**: Added new datasource for fetching TV show episodes from RTVE (Radio Televisión Española)
  - Fetches latest episodes from RTVE on-demand shows
  - Configurable show ID and maximum number of episodes
  - Includes subtitle availability and language information
  - Web renderer with RTVE branding and responsive design
  - Uses rtve-go v0.2.0 library with off-by-one bug fix

### 🐛 Bug Fixes

- **Init Command**: `ergs init` now skips overwriting existing configuration files instead of replacing them
- **RTVE-Go Library**: Fixed off-by-one bug in `FetchShowLatest` that was fetching maxVideos+1 instead of maxVideos
  - Added comprehensive unit tests to prevent regression

---

## [1.6.0] - 2025-08-05

### ✨ New Features

- **Date Filtering**: Added date range filtering to search with `start_date` and `end_date` parameters (YYYY-MM-DD format)
- **Advanced Search**: Collapsible advanced search section in web interface with date pickers, datasource filters, and results per page selector

---

## [1.5.0] - 2025-08-05

### ✨ New Features

- **Faster search** across many datasources

---

## [1.4.4] - 2025-08-04

### ✨ New Features

- **Web Interface Favicon**: Added favicon to web interface using the existing ergs logo
  - Optimized favicon sizes (16x16, 32x32) for different display contexts
  - Proper ICO file format with multiple embedded sizes for maximum browser compatibility
  - Modern PNG fallbacks for high-DPI displays

---

## [1.4.3] - 2025-08-04

### 🔧 Maintenance

- Version bump for release stability

---

## [1.4.2] - 2025-08-04

### 🔧 Improvements

- **CGO-Free Builds**: Switched to ncruces/go-sqlite3 driver to eliminate CGO dependency
  - Simplified build process and cross-compilation
  - Improved portability across different platforms
  - Better integration with Go toolchain

### 📚 Documentation

- Updated documentation to mention pre-built binaries availability

---

## [1.4.1] - 2025-08-04

### 🚀 Release Infrastructure

- **Automated Releases**: Added GoReleaser support for automated binary builds
  - GitHub Actions workflow for cross-platform releases
  - Automated binary generation for multiple architectures
  - Streamlined release process

### 📚 Documentation

- Enhanced configuration reload documentation

---

## [1.4.0] - 2025-01-04

### ✨ New Features

- **Configuration Reload**: Dynamic configuration reloading without service restart
  - **Automatic file watching**: Config changes detected automatically using filesystem events
  - **SIGHUP signal support**: Manual reload via Unix signals (`kill -HUP <pid>`)
  - **Complete refresh**: All datasources removed and re-added for consistency
  - **Error handling**: Invalid configs don't break running service
  - **Integration tests**: Comprehensive test coverage for both reload methods

### 🔧 Improvements

- **Enhanced serve command**: Now watches config file and responds to SIGHUP signals
- **Dynamic datasource management**: Add/remove/update datasources without restart
- **Better user experience**: Simply edit and save config file for automatic reload
- **Robust error recovery**: Service continues running if reload fails

### 🚀 Usage

```bash
# Start daemon (automatically watches config file)
ergs serve

# Option 1: Edit config file - automatic reload!
nano ~/.config/ergs/config.toml

# Option 2: Manual reload via signal
kill -HUP $(pgrep ergs)
```

---

## [1.3.0] - 2024-01-XX

### ✨ New Features

- **Web Interface**: Added modern web UI accessible via `ergs web`
  - Browse and search all datasources with responsive design
  - Real-time pagination and filtering (30 items per page)
  - Specialized renderers for each datasource type (GitHub, Firefox, HN, RSS, etc.)

- **REST API**: JSON endpoints for programmatic access
  - `/api/datasources` - List datasources
  - `/api/search` - Search across all data
  - `/api/datasources/{name}` - Browse specific datasource

### 🔧 Improvements

- **Modern Templates**: Migrated to templ for type-safe server-side rendering
- **Code Cleanup**: Removed 800+ lines of unused code and fixed all linting issues
- **Enhanced Documentation**: Added comprehensive web interface and API guides

### 🚀 Usage

```bash
# Start web interface
ergs web --port 8080

# Access at http://localhost:8080
```

---

## [1.2.0] and earlier

See git history for previous releases.
