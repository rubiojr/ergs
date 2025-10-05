# Docker Deployment Guide

This guide explains how to run Ergs using Docker and Docker Compose.

## Quick Start

1. Set your own importer api key in `docker/config.toml` or disable the importer removing the importer section and the importer datasource from `docker/config.toml`.

2. **Start the services**:
   ```bash
   cd docker
   docker compose up -d
   ```

3. **Access the web interface**:
   Open http://localhost:7117 in your browser

## Architecture

The Docker Compose setup runs two containers:

- **ergs-serve**: Scheduler daemon that fetches data from datasources at configured intervals
- **ergs-web**: Web interface for searching and browsing data
- **ergs-importer**: Service to receive blocks from external importers

All containers share the same data volume (`ergs-data`) where the SQLite database are stored.
