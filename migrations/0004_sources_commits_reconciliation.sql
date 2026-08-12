-- Source roots, a commit projection, and relations that do not belong to an
-- identity policy.
--
-- Two sources can observe the same bytes: the local-folder connector sees a
-- path with an mtime, the Git connector sees a path inside a commit. Nothing
-- could relate them, because a locator is only meaningful relative to its
-- source's root and roots were never recorded.

BEGIN;

-- Where each source's locators are rooted. This is what makes a locator
-- resolvable to a filesystem path, and therefore what lets two sources be
-- recognised as observing one file.
CREATE TABLE IF NOT EXISTS sources (
    source_id   TEXT PRIMARY KEY,
    kind        TEXT        NOT NULL,
    root        TEXT        NOT NULL,
    first_seen  TIMESTAMPTZ NOT NULL DEFAULT now(),
    last_seen   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Commits are artifacts but not files, so they get their own projection rather
-- than being forced into current_files.
CREATE TABLE IF NOT EXISTS commits (
    source_id    TEXT        NOT NULL,
    oid          TEXT        NOT NULL,
    seq          BIGINT      NOT NULL,
    observation_id TEXT      NOT NULL,
    author       TEXT        NOT NULL,
    message      TEXT        NOT NULL,
    authored_at  TIMESTAMPTZ NOT NULL,
    parents      TEXT[]      NOT NULL DEFAULT '{}',
    PRIMARY KEY (source_id, oid)
);

CREATE INDEX IF NOT EXISTS commits_authored_idx ON commits (authored_at DESC);

-- One row per path touched per commit. The unit of reconciliation: this is what
-- gets matched against a file observed by a filesystem scan.
CREATE TABLE IF NOT EXISTS commit_files (
    source_id  TEXT   NOT NULL,
    oid        TEXT   NOT NULL,
    locator    TEXT   NOT NULL,
    PRIMARY KEY (source_id, oid, locator),
    FOREIGN KEY (source_id, oid) REFERENCES commits(source_id, oid) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS commit_files_locator_idx ON commit_files (source_id, locator);

-- Relations derived from source-native evidence -- a commit touching a file --
-- do not depend on an identity profile, and forcing them to name one would
-- imply a grouping decision that was never made.
--
-- The primary key moves to relation_id alone, which is already a hash over the
-- policy version, type, and endpoints, so it stays unique.
ALTER TABLE relations DROP CONSTRAINT IF EXISTS relations_pkey;
ALTER TABLE relations DROP CONSTRAINT IF EXISTS relations_identity_policy_version_fkey;
ALTER TABLE relations ALTER COLUMN identity_policy_version DROP NOT NULL;
ALTER TABLE relations ADD PRIMARY KEY (relation_id);

COMMIT;
