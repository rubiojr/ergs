# Changelog

## [2.0.1] - 2025-10-04

### ✨ New Features

- **RTVE Datasource**: Added new datasource for fetching TV show episodes from RTVE (Radio Televisión Española)
  - Fetches latest episodes from RTVE on-demand shows
  - Configurable show ID and maximum number of episodes
  - Includes subtitle availability and language information
  - Web renderer with RTVE branding and responsive design

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
