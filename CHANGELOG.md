# Changelog

All notable changes to Ergs will be documented in this file.

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