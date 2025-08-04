# Changelog

All notable changes to Ergs will be documented in this file.

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