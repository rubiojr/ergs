-- Migration 003: Add FTS5 synchronization triggers for blocks_fts
--
-- Purpose:
--   Automate keeping the external-content FTS5 table (blocks_fts) in sync with the
--   base table (blocks) so the application no longer needs to manually maintain the
--   index after writes.
--
-- Context / Prior Migrations:
--   001_initial_schema.sql  -> created blocks + blocks_fts (external content mode)
--   002_add_hostname.sql    -> added hostname column and rebuilt blocks_fts including hostname
--
-- This migration installs three AFTER triggers on the blocks table:
--   - INSERT: add the new row to blocks_fts
--   - DELETE: issue a 'delete' command for the old rowid
--   - UPDATE: delete the old rowid then insert the new row
--
-- External Content FTS5 Pattern (per SQLite docs):
--   INSERT INTO blocks_fts(rowid, col1, ...) VALUES (new.rowid, new.col1, ...);
--   INSERT INTO blocks_fts(blocks_fts, rowid) VALUES('delete', old.rowid);
--
-- Safety & Idempotency:
--   - We DROP TRIGGER IF EXISTS before creating each trigger to allow the migration
--     to be re-run harmlessly in test scenarios (production code applies migrations once).
--   - A REBUILD is issued to ensure index consistency (cheap if already consistent).
--   - An integrity check is run to surface potential issues early (logged only on error).
--
-- Rollback:
--   - No rollback script is provided (forward-only). To remove triggers manually:
--       DROP TRIGGER IF EXISTS blocks_ai_fts;
--       DROP TRIGGER IF EXISTS blocks_ad_fts;
--       DROP TRIGGER IF EXISTS blocks_au_fts;
--
-- Assumptions:
--   - All referenced columns (text, source, datasource, metadata, hostname) exist.
--   - blocks_fts schema currently matches those columns (from migration 002).
--
-- ---------------------------------------------------------------------------
-- 1. Drop existing triggers if they exist (idempotent setup)
-- ---------------------------------------------------------------------------
DROP TRIGGER IF EXISTS blocks_ai_fts;
DROP TRIGGER IF EXISTS blocks_ad_fts;
DROP TRIGGER IF EXISTS blocks_au_fts;

-- ---------------------------------------------------------------------------
-- 2. Create AFTER INSERT trigger
-- ---------------------------------------------------------------------------
CREATE TRIGGER blocks_ai_fts
AFTER INSERT ON blocks
BEGIN
    INSERT INTO blocks_fts(rowid, text, source, datasource, metadata, hostname)
    VALUES (new.rowid, new.text, new.source, new.datasource, new.metadata, new.hostname);
END;

-- ---------------------------------------------------------------------------
-- 3. Create AFTER DELETE trigger
-- ---------------------------------------------------------------------------
CREATE TRIGGER blocks_ad_fts
AFTER DELETE ON blocks
BEGIN
    INSERT INTO blocks_fts(blocks_fts, rowid) VALUES('delete', old.rowid);
END;

-- ---------------------------------------------------------------------------
-- 4. Create AFTER UPDATE trigger
-- ---------------------------------------------------------------------------
CREATE TRIGGER blocks_au_fts
AFTER UPDATE ON blocks
BEGIN
    -- Remove old version
    INSERT INTO blocks_fts(blocks_fts, rowid) VALUES('delete', old.rowid);
    -- Add new version
    INSERT INTO blocks_fts(rowid, text, source, datasource, metadata, hostname)
    VALUES (new.rowid, new.text, new.source, new.datasource, new.metadata, new.hostname);
END;

-- ---------------------------------------------------------------------------
-- 5. Rebuild FTS index to guarantee a clean baseline (safe & idempotent)
-- ---------------------------------------------------------------------------
INSERT INTO blocks_fts(blocks_fts) VALUES('rebuild');

-- (Optional) Integrity check - failures will surface as migration errors if the
-- SQLite library returns an error. This can help detect latent corruption early.
INSERT INTO blocks_fts(blocks_fts) VALUES('integrity-check');

-- (Optional) Optimize – can improve performance; ignored if not beneficial.
INSERT INTO blocks_fts(blocks_fts) VALUES('optimize');

-- End of migration 003
