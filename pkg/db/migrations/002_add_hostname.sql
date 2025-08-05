-- Migration 002: Add hostname to blocks and update FTS index using a replacement table approach
--
-- This migration assumes that the "blocks" table already exists from migration 001_initial_schema.sql.

-- Step 1: Add the new "hostname" column to the base table.
ALTER TABLE blocks ADD COLUMN hostname TEXT;

-- Step 2: Create a new replacement FTS table that includes the hostname column.
CREATE VIRTUAL TABLE blocks_fts_new USING fts5(
    text,
    source,
    datasource,
    metadata,
    hostname,
    content='blocks',
    content_rowid='rowid',
    tokenize='porter'
);

-- Step 5: Rebuild the FTS index from the external content table (blocks).
-- This is the correct approach for external content tables.
INSERT INTO blocks_fts_new(blocks_fts_new) VALUES('rebuild');

-- Step 7: Atomically swap the old FTS table with the new one.
DROP TABLE IF EXISTS blocks_fts;
ALTER TABLE blocks_fts_new RENAME TO blocks_fts;
