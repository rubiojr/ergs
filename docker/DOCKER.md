# Docker Deployment Guide

This guide explains how to run Ergs using Docker and Docker Compose.

## Quick Start

1. **Create a configuration file**:
   ```bash
   cp pkg/config/config.toml.sample config.toml
   ```

2. **Edit `config.toml`** with your datasource configurations:
   ```toml
   storage_dir = '/data' # this is required path

   # tweak datasources to your liking
   [datasources.github]
   type = 'github'
   interval = '30m0s'
   [datasources.github.config]
   token = 'your-github-token'
   ```

3. **Start the services**:
   ```bash
   docker compose up -d
   ```

4. **Access the web interface**:
   Open http://localhost:8080 in your browser

## Architecture

The Docker Compose setup runs two containers:

- **ergs-serve**: Scheduler daemon that fetches data from datasources at configured intervals
- **ergs-web**: Web interface for searching and browsing data

Both containers share the same data volume (`ergs-data`) where the SQLite database is stored.

## Configuration

### Data Volume

The `ergs-data` volume is shared between both containers and stores:
- SQLite database (`ergs.db`)
- Any other persistent data

Data persists across container restarts and recreations.

### Port Mapping

By default, the web interface is exposed on port 8080. To change this, edit `compose.yml`:

```yaml
services:
  ergs-web:
    ports:
      - "3000:8080"  # Change host port to 3000
```

### Environment Variables

You can override configuration using environment variables in `compose.yml`:

```yaml
services:
  ergs-serve:
    environment:
      - TZ=America/New_York  # Set timezone
```

### Custom Storage Directory

The `storage_dir` in your config should be set to `/data` (the container path). The Docker Compose file mounts this as a volume.

## Common Operations

### View Logs

```bash
# View all logs
docker compose logs -f

# View specific service logs
docker compose logs -f ergs-web
docker compose logs -f ergs-serve
```

### Restart Services

```bash
# Restart all services
docker compose restart

# Restart specific service
docker compose restart ergs-web
```

### Update Configuration

1. Edit `config.toml`
2. Restart the services:
   ```bash
   docker compose restart
   ```

### Initialize Configuration

If you need to run `ergs init`:

```bash
docker compose run --rm ergs-serve ergs init --config /config/config.toml
```

### Manual Data Fetch

Trigger a manual fetch without waiting for the scheduler:

```bash
docker compose exec ergs-serve ergs fetch --config /config/config.toml
```

### Search from CLI

```bash
docker compose exec ergs-web ergs search --config /config/config.toml --query "your search"
```

### Access Database Directly

```bash
# Get a shell in the container
docker compose exec ergs-web sh

# Then access the database
sqlite3 /data/ergs.db
```

## Building the Image

The `compose.yml` file builds the image automatically. To build manually:

```bash
docker build -t ergs:latest .
```

## Production Deployment

### Using HTTPS

For production, you should use a reverse proxy with SSL/TLS:

**Example with Nginx**:

```nginx
server {
    listen 443 ssl http2;
    server_name ergs.example.com;

    ssl_certificate /path/to/cert.pem;
    ssl_certificate_key /path/to/key.pem;

    location / {
        proxy_pass http://localhost:8080;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;
    }
}
```

**Example with Caddy** (automatic HTTPS):

```
ergs.example.com {
    reverse_proxy localhost:8080
}
```

### Resource Limits

Add resource limits in `compose.yml`:

```yaml
services:
  ergs-serve:
    deploy:
      resources:
        limits:
          cpus: '1'
          memory: 512M
        reservations:
          cpus: '0.5'
          memory: 256M
```

### Health Checks

The containers include health checks. View status:

```bash
docker compose ps
```

Healthy services will show `(healthy)` in the status.

## Backup and Restore

### Backup Data

```bash
# Create backup directory
mkdir -p backups

# Backup the database
docker compose exec ergs-web sh -c 'sqlite3 /data/ergs.db ".backup /data/backup.db"'
docker cp ergs-web:/data/backup.db backups/ergs-$(date +%Y%m%d).db
```

### Restore Data

```bash
# Stop services
docker compose down

# Copy backup to volume
docker compose run --rm -v $(pwd)/backups/ergs-20241004.db:/backup.db ergs-serve sh -c 'cp /backup.db /data/ergs.db'

# Start services
docker compose up -d
```

## Troubleshooting

### Container Won't Start

Check logs:
```bash
docker compose logs ergs-serve
docker compose logs ergs-web
```

### Configuration Issues

Verify config is mounted correctly:
```bash
docker compose exec ergs-web cat /config/config.toml
```

### Database Locked

If you get "database is locked" errors:
```bash
# Stop all containers
docker compose down

# Start again
docker compose up -d
```

### Disk Space

Check volume size:
```bash
docker system df -v
```

Clean up unused resources:
```bash
docker system prune -a
```

## Updating Ergs

1. Pull latest code:
   ```bash
   git pull
   ```

2. Rebuild and restart:
   ```bash
   docker compose down
   docker compose build --no-cache
   docker compose up -d
   ```

## Example Configuration

Complete `config.toml` example for Docker:

```toml
storage_dir = '/data'

[datasources.github]
type = 'github'
interval = '30m0s'
[datasources.github.config]
token = 'ghp_your_token_here'

[datasources.hackernews]
type = 'hackernews'
interval = '1h0m0s'
[datasources.hackernews.config]
fetch_top = true
max_items = 50

[datasources.rtve]
type = 'rtve'
interval = '2h0m0s'
[datasources.rtve.config]
show_id = 'telediario-1'
max_episodes = 10
```

## Security Notes

1. **Don't commit config.toml**: Add it to `.gitignore` if it contains tokens
2. **Use secrets**: For production, consider Docker secrets:
   ```yaml
   secrets:
     github_token:
       file: ./secrets/github_token.txt
   ```
3. **Network isolation**: Containers are on the default bridge network
4. **Non-root user**: Containers run as user `ergs` (UID 1000)

## Support

For issues or questions:
- Check logs: `docker compose logs`
- View container status: `docker compose ps`
- Restart services: `docker compose restart`
