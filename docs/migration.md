# Database Migrations in Ergs

## Overview

Ergs uses a robust database migration system to evolve the schema over time while maintaining data integrity and backward compatibility. This document explains how migrations work, when they're needed, and how to manage them.

## What Are Migrations?

Migrations are versioned SQL scripts that incrementally modify the database schema. Each migration is applied exactly once and tracked in a `migrations` table. This ensures that all instances of Ergs have the same database structure regardless of when they were created or updated.

## Migration System Architecture

### Key Components

1. **Migration Manager** (`pkg/db/migrations.go`)
   - Manages migration discovery, application, and tracking
   - Supports both embedded migrations (production) and directory-based migrations (testing)

2. **Migration Files** (`pkg/db/migrations/*.sql`)
   - Numbered SQL files (e.g., `001_initial_schema.sql`, `002_add_hostname.sql`)
   - Embedded in the binary for production use

3. **Migrations Table**
   - Tracks which migrations have been applied
   - Schema: `(version INTEGER PRIMARY KEY, applied_at DATETIME)`

4. **Storage Manager Integration**
   - Automatically checks for pending migrations
   - Provides migration-aware storage access

### Migration Lifecycle

```
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│   New Install   │    │  Existing Install │    │   After Update  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
         │                       │                       │
         v                       v                       v
┌─────────────────┐    ┌──────────────────┐    ┌─────────────────┐
│ Auto-apply all  │    │ Check pending    │    │ Require manual  │
│ migrations for  │    │ migrations on    │    │ migration via   │
│ new databases   │    │ startup          │    │ 'ergs migrate'  │
└─────────────────┘    └──────────────────┘    └─────────────────┘
```

## Current Migrations

### Migration 1: Initial Schema (`001_initial_schema.sql`)

Creates the foundational database structure:

- **`blocks`** table: Stores all fetched content blocks
- **`fetch_metadata`** table: Tracks fetch timestamps and metadata
- **`blocks_fts`** table: Full-text search index using SQLite FTS5
- **`migrations`** table: Migration tracking

### Migration 2: Add Hostname (`002_add_hostname.sql`)

Adds hostname tracking for better multi-machine deployments using a safe replacement table approach:

- Adds `hostname` column to `blocks` table
- Creates new FTS table with hostname field using replacement table strategy
- Uses dual-write triggers during migration to capture concurrent writes
- Rebuilds FTS index using proper `rebuild` command for external content tables
- Performs atomic table swap to minimize downtime
- Cleans up temporary triggers after migration

**Safety Features:**
- **No downtime**: Old FTS table remains available during migration
- **Memory safe**: Uses FTS5's internal `rebuild` command instead of manual copying
- **Concurrent write safety**: Dual-write triggers ensure no data loss
- **Atomic switchover**: Table rename is instantaneous

### Migration 3: FTS Triggers (`003_add_fts_triggers.sql`)

Automates synchronization of the external-content FTS5 table (`blocks_fts`) with the base `blocks` table so application code no longer needs to manually maintain the index after writes.

Added components:
- AFTER INSERT trigger (`blocks_ai_fts`) inserts new rows into `blocks_fts`
- AFTER DELETE trigger (`blocks_ad_fts`) issues the FTS5 delete command for removed rows
- AFTER UPDATE trigger (`blocks_au_fts`) deletes the old rowid entry then inserts the updated row

Pattern used (SQLite FTS5 external content maintenance):
- Insert: `INSERT INTO blocks_fts(rowid, text, source, datasource, metadata, hostname) VALUES (new.rowid, ...)`
- Delete: `INSERT INTO blocks_fts(blocks_fts, rowid) VALUES('delete', old.rowid)`

Post‑installation steps inside the migration:
1. Rebuild the index: `INSERT INTO blocks_fts(blocks_fts) VALUES('rebuild')`
2. Integrity check: `INSERT INTO blocks_fts(blocks_fts) VALUES('integrity-check')`
3. Optimize: `INSERT INTO blocks_fts(blocks_fts) VALUES('optimize')`

Safety / Idempotency:
- Triggers are dropped first (`DROP TRIGGER IF EXISTS ...`) to allow safe reapplication in controlled test environments.
- Rebuild + integrity + optimize are no-ops if already consistent.
- Migration assumes schema from 002 (i.e. `hostname` already present).

Operational Impact:
- Removes need for manual FTS population on each write path.
- Ensures search results reflect mutations immediately after commit.
- Maintains external content mode advantages (single source of truth in `blocks`).

Future Considerations:
- If additional searchable columns are added later, a follow-up migration must (a) recreate or extend triggers if columns are referenced, and (b) rebuild the FTS index again.
- If performance tuning is required, triggers could be batched or replaced with a deferred job system in a later migration.

### Migration 4: Add updated_at (`004_add_updated_at.sql`)

Adds an `updated_at` column to distinguish the original creation time (`created_at`) from the last modification time. This enables:
- Incremental syncs (e.g. “give me blocks changed since T”)
- Efficient cache invalidation
- Audit/troubleshooting of mutation behavior

Key changes:
1. `ALTER TABLE blocks ADD COLUMN updated_at DATETIME`
2. Backfill: `updated_at = created_at` for all existing rows
3. Index: `CREATE INDEX IF NOT EXISTS idx_blocks_updated_at ON blocks(updated_at)`

Design notes:
- We deliberately do NOT add `updated_at` to the FTS virtual table; it is not part of search relevance scoring.
- The application upsert logic changes so:
  - On first insert: both `created_at` and `updated_at` are set to the block’s creation moment.
  - On conflict (same id): `created_at` is preserved, `updated_at` is set to `CURRENT_TIMESTAMP`.
- No trigger is used for `updated_at` to keep control in application code and avoid recursive trigger complexity.

Rationale:
- Preserving `created_at` keeps the lineage/original event time intact.
- Many datasources may re-fetch or enrich existing blocks; this surfaces genuine mutations without losing origin time.

Potential downsides:
- Extra write amplification on frequent updates (one more indexed column to maintain).
- Storage growth: the new index adds a modest B-tree (generally small relative to text content).
- For immutable datasets, `updated_at` duplicates `created_at` (harmless; consumers can ignore or coalesce).

Future options:
- If automatic timestamping is desired later, a follow-up migration could add an AFTER UPDATE trigger to set `updated_at = CURRENT_TIMESTAMP` only when relevant columns change.
- If per-field change tracking is required, consider adding a lightweight changelog table in a later migration.

Operational impact:
- Existing queries unaffected unless they explicitly select all columns with positional assumptions.
- Clients can begin using `updated_at` immediately after migration 4 is applied.

### Migration 5: Add ingested_at (`005_add_ingested_at.sql`)

Adds an `ingested_at` column to track when blocks were first added to Ergs, separate from:
- `created_at`: when the underlying data/content was originally created (from source)
- `updated_at`: when the block was last modified in Ergs

This new timestamp enables:
- Understanding data freshness from Ergs' perspective
- Debugging ingestion workflows (when did we first see this?)
- Analytics on data collection timelines
- Distinguishing between content age and ingestion recency

Key changes:
1. `ALTER TABLE blocks ADD COLUMN ingested_at DATETIME`
2. Backfill: `ingested_at = updated_at` for all existing rows (best approximation available)
3. Index: `CREATE INDEX IF NOT EXISTS idx_blocks_ingested_at ON blocks(ingested_at)`

Semantic Breakdown (after this migration):
- **created_at**: When the source data was created (GitHub event time, RSS pubDate, etc.)
- **ingested_at**: When Ergs first stored this block (set on first INSERT, never changes)
- **updated_at**: When Ergs last modified this block (updates on conflict/change)

Design notes:
- We do NOT add `ingested_at` to the FTS virtual table (not needed for search)
- Application upsert logic updated:
  - On INSERT: set `ingested_at = CURRENT_TIMESTAMP`
  - ON CONFLICT: preserve `ingested_at` (never overwrite)
- For existing blocks, `updated_at` serves as a reasonable proxy for ingestion time

Rationale:
- Many use cases need to know "when did Ergs collect this?" independent of "when was this created?"
- Example: A GitHub event from 2020 ingested today should show created_at=2020 but ingested_at=today
- Enables queries like "show me everything ingested in the last hour" regardless of content age

Considerations:
- Adds one more indexed column (slight storage/write overhead)
- **Backfill accuracy limitations:**
  - Blocks that existed before migration 004: `ingested_at = updated_at = created_at` (loses true ingestion time)
  - Blocks created between migrations 004-005: `ingested_at ≈ actual ingestion time` (reasonably accurate)
  - Blocks created after migration 005: `ingested_at = accurate ingestion time` (fully accurate)
- This is the best available approximation given that migration 004 backfilled `updated_at = created_at`
- For truly accurate ingestion tracking, this field is only reliable for blocks added after this migration

Future options:
- The `ingested_at` field is tracked in the database but not yet exposed through the API
- Future work may add it to block responses if needed for client use cases
- Could enable features like "recently ingested" feeds or ingestion rate analytics
- Note: For historical analysis, only blocks ingested after this migration have accurate timestamps

Operational impact:
- Existing queries unaffected (column is added, not required)
- New blocks automatically track ingestion time
- Existing blocks have reasonable approximation from `updated_at`


## Using Migrations

### Check Migration Status

```bash
# Check status for all datasources
ergs migrate --status

# Check status for specific datasource
ergs migrate --status --datasource github
```

Status output shows:
- Applied migrations with timestamps
- Pending migrations that need to be applied
- Database state (up-to-date or requires migration)

### Apply Migrations

```bash
# Apply all pending migrations
ergs migrate

# Apply migrations for specific datasource
ergs migrate --datasource github
```

**Important**: The `migrate` command is the only way to apply migrations to existing databases. Other commands will fail if pending migrations exist.

#### FTS Index Management

Migration 2 uses a safe replacement table approach:

- **Populated FTS after migration**: The FTS index is fully populated after migration using the `rebuild` command
- **Memory efficient**: Uses FTS5's internal rebuild process which handles memory management
- **No manual intervention needed**: All existing data is automatically searchable after migration
- **Production ready**: Safe for databases of any size

**Migration Process:**
1. Creates replacement FTS table with hostname column
2. Sets up dual-write triggers for concurrent writes
3. Uses `INSERT INTO blocks_fts_new(blocks_fts_new) VALUES('rebuild')` to populate index
4. Optimizes the new index
5. Atomically swaps old and new tables
6. Cleans up triggers and temporary tables

This approach ensures zero data loss and minimal downtime while maintaining full search functionality.

### Error Handling

If you try to use Ergs with pending migrations, you'll see:

```
Error: database 'github' has 1 pending migrations. Run 'ergs migrate' first
```

This prevents data corruption and ensures schema consistency.

## Migration Behavior

### New Installations

- Fresh databases automatically get the latest schema
- All migrations are applied during first storage access
- No manual migration step required

### Existing Installations

- Databases created with older versions require explicit migration
- Commands that access the database check for pending migrations
- Migration must be performed manually via `ergs migrate`

### Safety Features

- **Transactional**: Each migration runs in a transaction
- **Rollback on failure**: Failed migrations are rolled back automatically
- **Version tracking**: Applied migrations are tracked to prevent re-application
- **Read-only status**: `--status` flag never modifies the database

## Database Schema Evolution

### Design Principles

1. **Additive Changes**: Prefer adding columns over modifying existing ones
2. **Backward Compatibility**: Ensure old data remains accessible
3. **FTS Index Updates**: Update search indexes when adding searchable fields
4. **Default Values**: Provide sensible defaults for new columns

### Example Migration Pattern

```sql
-- Migration: Add new feature
-- File: 003_add_feature.sql

-- Add new column with default value
ALTER TABLE blocks ADD COLUMN new_field TEXT DEFAULT '';

-- Update FTS index to include new field
DROP TABLE IF EXISTS blocks_fts;
CREATE VIRTUAL TABLE blocks_fts USING fts5(
    text,
    source,
    datasource,
    metadata,
    hostname,
    new_field,  -- New searchable field
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);

-- Note: The migration uses a conservative approach to prevent memory issues
-- This provides maximum safety for databases of all sizes
-- 
-- The approach:
-- 1. Adds hostname column to blocks table
-- 2. Recreates FTS table with new schema (but leaves it empty)
-- 3. Prevents out-of-memory errors on large databases
-- 4. Application handles FTS sync for new data going forward
-- 5. Existing data can be indexed manually if needed
```

## Development and Testing

### Migration Testing

The migration system includes comprehensive tests:

- **Unit tests**: Individual migration manager functions
- **Integration tests**: End-to-end migration scenarios
- **CLI tests**: Black box testing of migration commands

### Test Isolation

Tests use directory-based migrations to avoid conflicts with production migrations:

```go
// Create test migration manager
migrationManager := db.NewMigrationManagerFromPath(database, testMigrationsDir)
```

### Adding New Migrations

1. Create numbered SQL file in `pkg/db/migrations/`
2. Use sequential numbering (e.g., `003_description.sql`)
3. Include comprehensive SQL comments
4. Test with both new and existing databases
5. Update this documentation

## Troubleshooting

### Common Issues

**"pending migrations detected"**
- Run `ergs migrate --status` to see what needs to be applied
- Run `ergs migrate` to apply pending migrations

**"out of memory" during migration**
- Migration 2 now avoids all memory issues by leaving FTS index empty
- This is the intended behavior to prevent crashes on large databases
- The migration focuses on schema changes only, not data migration

**"Search returns no results" after migration**
- This should not happen with the new migration approach - FTS index is fully populated
- If you see this, check that the migration completed successfully
- Verify FTS index integrity: `INSERT INTO blocks_fts(blocks_fts) VALUES('integrity-check');`
- Check migration status: `ergs migrate --status`

**"new data not appearing in search results"**
- The application handles FTS synchronization in `StoreBlocks()`, not triggers
- Migration 2 removes temporary triggers after completing the rebuild
- Check that the application is properly inserting into both `blocks` and `blocks_fts` tables
- For manual FTS sync: `INSERT INTO blocks_fts(...) SELECT ... FROM blocks WHERE ...`

**Migration fails partway through**
- Failed migrations are automatically rolled back
- Check error message for SQL syntax issues
- Fix migration file and retry

### Recovery Scenarios

**Corrupted migrations table**
```sql
-- Recreate migrations table
DROP TABLE IF EXISTS migrations;
CREATE TABLE migrations (
    version INTEGER PRIMARY KEY,
    applied_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
-- Manually insert applied migration records
```

**FTS index corruption or inconsistency**
```sql
-- Check if FTS index exists
SELECT name FROM sqlite_master WHERE type='table' AND name='blocks_fts';

-- Check how much data is in FTS index
SELECT COUNT(*) FROM blocks_fts;

-- Check how much data should be in FTS index
SELECT COUNT(*) FROM blocks;

-- Rebuild FTS index (safe with external content tables)
INSERT INTO blocks_fts(blocks_fts) VALUES('rebuild');

-- Check FTS index integrity
INSERT INTO blocks_fts(blocks_fts) VALUES('integrity-check');

-- If rebuild fails on very large databases, consider the replacement table approach:
-- CREATE VIRTUAL TABLE blocks_fts_new USING fts5(...);
-- INSERT INTO blocks_fts_new(blocks_fts_new) VALUES('rebuild');
-- -- Then swap tables as shown in migration 002
```

**Schema out of sync**
- Compare actual schema with expected migration results
- Consider creating corrective migration
- In extreme cases, backup data and recreate database

## Best Practices

### For Users

1. **Always backup** before running migrations
2. **Test migrations** on development data first
3. **Check status** before and after migration
4. **Monitor logs** during migration process

### For Developers

1. **Test thoroughly** with various database states
2. **Use transactions** for all schema changes
3. **Provide rollback** information in comments
4. **Document breaking changes** clearly

### For Operators

1. **Schedule migrations** during maintenance windows
2. **Monitor disk space** during large migrations
3. **Verify results** after migration completion
4. **Keep backups** of pre-migration state

## Future Considerations

### Planned Enhancements

- **Migration rollback**: Automated rollback capabilities
- **Dry run mode**: Preview migration effects without applying
- **Migration validation**: Pre-flight checks for migration safety
- **Schema diffing**: Compare expected vs. actual schema

### Migration Strategy

- Maintain backward compatibility for at least 2 major versions
- Deprecate features before removing them
- Provide clear migration paths for breaking changes
- Consider data volume impact on migration performance

## Related Documentation

- [Architecture Overview](architecture.md) - System architecture including storage layer
- [Configuration Guide](configuration.md) - Database configuration options
- [Development Guide](development.md) - Setting up development environment
- [Troubleshooting Guide](troubleshooting.md) - Common issues and solutions