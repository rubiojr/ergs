# Configuration Reload

Ergs supports dynamic configuration reloading without stopping the service through two mechanisms:

## Automatic File Watching (Recommended)

Ergs automatically watches the configuration file for changes and reloads when modifications are detected. Simply edit and save your config file - no manual signals required.

## Manual SIGHUP Signal

You can also manually trigger a reload by sending a SIGHUP signal to the ergs process.

Both methods will:

1. Re-read the configuration file
2. Remove all existing datasources
3. Add all datasources from the new configuration
4. Continue running with the updated configuration

This allows you to add, remove, or modify datasources without restarting the ergs daemon.

## Usage

### Starting the Server

Start the ergs daemon:

```bash
ergs serve
```

You'll see output like:
```
Starting warehouse with per-datasource intervals
Warehouse started. Press Ctrl+C to stop, send SIGHUP to reload, or modify config file for automatic reload.
Watching config file for changes: /home/user/.config/ergs/config.toml
```

### Automatic Reload (Recommended)

Simply edit and save your configuration file. Ergs will automatically detect the change and reload:

```bash
# Edit your config file
nano ~/.config/ergs/config.toml

# Save the file - reload happens automatically!
```

### Manual Reload with SIGHUP

Alternatively, send a SIGHUP signal to the ergs process:

```bash
# Find the process ID
ps aux | grep ergs

# Send SIGHUP signal
kill -HUP <pid>

# Or use pkill
pkill -HUP ergs
```

### Log Output

When reloading automatically via file watching:

```
Config file changed: /home/user/.config/ergs/config.toml, reloading configuration...
Removing datasource: old-github
Removing datasource: old-firefox
Adding datasource: new-github
Adding datasource: new-codeberg
Configuration reload complete: removed 2 datasources, added 2 datasources
Configuration reloaded successfully after file change
```

When reloading manually via SIGHUP:

```
Received SIGHUP, reloading configuration...
Removing datasource: old-github
Removing datasource: old-firefox
Adding datasource: new-github
Adding datasource: new-codeberg
Configuration reload complete: removed 2 datasources, added 2 datasources
Configuration reloaded successfully
```

## What Gets Reloaded

The reload mechanism handles:

- **Adding new datasources**: Any datasources in the new config that weren't in the old config
- **Removing datasources**: Any datasources that were removed from the config
- **Configuration changes**: Modified settings for existing datasources (tokens, intervals, etc.)
- **Interval changes**: Updated fetch intervals for datasources

## Configuration Examples

### Initial Configuration

```toml
storage_dir = '/home/user/.local/share/ergs'

[datasources.github-work]
type = 'github'
interval = '30m0s'
[datasources.github-work.config]
token = 'old-token'

[datasources.firefox]
type = 'firefox'
[datasources.firefox.config]
database_path = '/path/to/firefox/places.sqlite'
```

### Updated Configuration

```toml
storage_dir = '/home/user/.local/share/ergs'

[datasources.github-personal]
type = 'github'
interval = '15m0s'  # Changed interval
[datasources.github-personal.config]
token = 'new-personal-token'  # New token

[datasources.codeberg]
type = 'codeberg'
[datasources.codeberg.config]
token = 'codeberg-token'

# firefox datasource removed
```

After sending SIGHUP:
- `github-work` datasource is removed
- `firefox` datasource is removed  
- `github-personal` datasource is added with new token and 15m interval
- `codeberg` datasource is added

## Error Handling

If the configuration reload fails:

1. **Invalid configuration**: The old configuration remains active
2. **Missing datasource types**: Only valid datasources are loaded
3. **Permission errors**: Logged as warnings, reload continues

Example error handling for automatic reload:
```
Config file changed: /home/user/.config/ergs/config.toml, reloading configuration...
Failed to reload configuration after file change: loading new config: parsing config: invalid TOML syntax
```

Example error handling for manual reload:
```
Received SIGHUP, reloading configuration...
Failed to reload configuration: loading new config: parsing config: invalid TOML syntax
```

The daemon continues running with the previous configuration.

## Best Practices

### 1. Test Configuration First

Before making changes (especially when using automatic reload), test your configuration:

```bash
# Test the configuration
ergs --config /path/to/new/config.toml datasource list
```

### 2. Temporary Files

When using editors that create temporary files, ergs might detect multiple file changes. This is normal and harmless.

### 3. Backup Current Configuration

Keep a backup of your working configuration:

```bash
cp ~/.config/ergs/config.toml ~/.config/ergs/config.toml.backup
```

### 4. Monitor Logs

Watch the logs during reload to ensure everything works:

```bash
# If running as systemd service
journalctl -u ergs -f

# If running in foreground
# Logs will appear in the terminal
```

### 5. Gradual Changes

Make incremental changes rather than wholesale configuration rewrites to make troubleshooting easier.

### 6. Editor Considerations

Some editors create temporary files or use atomic writes (rename operations). Ergs handles these gracefully:
- **Atomic writes**: Editors like vim, VS Code, and nano that rename temp files over the original are fully supported
- **File removal**: If the config file is deleted without replacement, reload is skipped to prevent errors
- **Multiple events**: Some editors may trigger multiple file system events; each is handled safely

## Integration with Process Managers

### systemd

If running ergs as a systemd service:

```bash
# Reload configuration
systemctl reload ergs

# Or send signal directly
systemctl kill -s HUP ergs
```

### Docker

For containerized deployments:

```bash
# Send signal to container
docker kill -s HUP <container-name>

# Or if using docker-compose
docker-compose kill -s HUP ergs
```

## Limitations

- **Storage directory changes**: Changes to `storage_dir` require a full restart
- **Running fetches**: In-progress data fetches are cancelled and restarted
- **Database connections**: Existing database connections are closed and reopened
- **File system events**: Some editors may trigger multiple reload events; this is harmless
- **Network file systems**: File watching may not work reliably on some network-mounted file systems
- **File removal**: If config file is deleted (not replaced), reload is skipped until file is recreated

## Troubleshooting

### Automatic Reload Not Working

1. Check if file watching is enabled in the logs:
   ```bash
   # Look for this message at startup
   Watching config file for changes: /path/to/config.toml
   ```

2. Verify the config file path is correct:
   ```bash
   ergs --help  # Check default config path
   ```

3. Test with manual file modification:
   ```bash
   touch ~/.config/ergs/config.toml
   ```

4. Check file system support (some network file systems don't support inotify)

### Manual Signal Not Working

1. Check if the process is still running:
   ```bash
   ps aux | grep ergs
   ```

2. Verify the process can receive signals:
   ```bash
   kill -0 <pid>
   ```

3. Check file permissions on the config file:
   ```bash
   ls -la ~/.config/ergs/config.toml
   ```

### Configuration Not Updating

1. Verify the config file path:
   ```bash
   ergs --help  # Check default config path
   ```

2. Check for syntax errors:
   ```bash
   # Use a TOML validator or
   ergs --config /path/to/config.toml datasource list
   ```

3. Monitor logs for error messages during reload

### Datasources Not Starting

1. Check datasource configuration format
2. Verify required fields are present
3. Test datasource creation manually:
   ```bash
   ergs --config /path/to/config.toml fetch --datasource <name>
   ```

### Multiple Reload Events

If you see multiple reload events from a single file save:

1. This is normal behavior with some editors
2. Each reload is harmless and will use the latest file content
3. Atomic writes (rename operations) are fully supported and handled correctly

### File Removal Handling

If the config file is removed:

1. **Atomic writes**: File replaced via rename - reload happens normally
2. **Actual deletion**: File removed without replacement - reload is skipped
3. **Recovery**: Recreate the config file to resume automatic reloading

### File Watching Errors

If you see "Config file watcher error" messages:

1. Check file system permissions
2. Verify the config directory exists
3. Ensure the file system supports inotify (Linux) or equivalent
4. Consider using manual SIGHUP reload as a fallback
