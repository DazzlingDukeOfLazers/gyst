-- Projects and membership, as a projection over manifest and marker evidence.
--
-- A project is not a folder. It is a saved set over the artifact graph, and
-- an artifact can belong to none, one, or several. Membership evidence has a
-- precedence (design-round-2.md section 2): an explicit assertion, then a
-- checked-in .gyst/project.yaml, then a source-native marker such as a .git
-- directory, then organization rules, then Gyst's own suggestions. The basis
-- column records which of those a row rests on, and the confidence is honest
-- about it: a manifest declares, a marker only suggests.
--
-- Everything here is rebuilt from the observation log on every projection
-- pass. Nothing is authoritative; the manifests and markers in the sources
-- are.

BEGIN;

CREATE TABLE IF NOT EXISTS projects (
    project_id  TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    basis       TEXT NOT NULL
                CHECK (basis IN ('explicit','manifest','native-marker','organization-rule','suggestion')),
    -- Where the declaring evidence was observed. A manifest project may be
    -- declared in several sources under one id; this is the first.
    source_id   TEXT NOT NULL,
    locator     TEXT NOT NULL,
    evidence    TEXT[] NOT NULL,
    confidence  REAL NOT NULL,
    explanation TEXT NOT NULL
);

-- What a project says it contains: glob patterns over locators in a source.
CREATE TABLE IF NOT EXISTS project_members (
    project_id  TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    source_id   TEXT NOT NULL,
    pattern     TEXT NOT NULL,
    basis       TEXT NOT NULL,
    confidence  REAL NOT NULL,
    evidence    TEXT NOT NULL,
    PRIMARY KEY (project_id, source_id, pattern)
);

-- The patterns applied to the current file inventory. One row per file per
-- project it belongs to; several rows per file are expected, not a conflict.
CREATE TABLE IF NOT EXISTS file_projects (
    source_id   TEXT NOT NULL,
    locator     TEXT NOT NULL,
    project_id  TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    basis       TEXT NOT NULL,
    confidence  REAL NOT NULL,
    pattern     TEXT NOT NULL,
    PRIMARY KEY (source_id, locator, project_id)
);

CREATE INDEX IF NOT EXISTS file_projects_project_idx ON file_projects (project_id);

COMMIT;
