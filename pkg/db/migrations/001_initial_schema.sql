-- Initial schema for Ergs database
-- This creates the base tables for storing blocks and metadata

-- Main blocks table stores all block data
CREATE TABLE IF NOT EXISTS blocks (
    id TEXT PRIMARY KEY,
    text TEXT NOT NULL,
    created_at DATETIME NOT NULL,
    source TEXT NOT NULL,
    datasource TEXT NOT NULL,
    metadata TEXT
);

-- Fetch metadata table stores datasource-specific metadata like last fetch times
CREATE TABLE IF NOT EXISTS fetch_metadata (
    key TEXT PRIMARY KEY,
    value TEXT NOT NULL,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);

-- Full-text search index for blocks
CREATE VIRTUAL TABLE IF NOT EXISTS blocks_fts USING fts5(
    text,
    source,
    datasource,
    metadata,
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);

-- Migration tracking table
CREATE TABLE IF NOT EXISTS migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);