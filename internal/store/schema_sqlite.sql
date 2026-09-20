-- SQLite schema: the eleven PostgreSQL migrations consolidated into the
-- shape they reach, in SQLite's types. Timestamps are TEXT in the fixed
-- layout the engine writes, so ordering is textual and correct. JSON is
-- TEXT. BIGSERIAL is INTEGER PRIMARY KEY AUTOINCREMENT. The triggers say
-- what the PL/pgSQL ones say.

CREATE TABLE IF NOT EXISTS observations (
    seq                   INTEGER PRIMARY KEY AUTOINCREMENT,
    observation_id        TEXT    NOT NULL UNIQUE,
    schema_version        TEXT    NOT NULL,
    source_id             TEXT    NOT NULL,
    connector             TEXT    NOT NULL,
    connector_version     TEXT    NOT NULL,
    cursor                TEXT,
    observed_at           TEXT    NOT NULL,
    recorded_at           TEXT    NOT NULL,
    subject_kind          TEXT    NOT NULL,
    locator               TEXT    NOT NULL,
    native_version_scheme TEXT    NOT NULL,
    native_version_value  TEXT    NOT NULL,
    content_digest_algo   TEXT,
    content_digest_hex    TEXT,
    size_bytes            INTEGER,
    claim_type            TEXT    NOT NULL,
    claim_payload         TEXT    NOT NULL,
    extractor             TEXT    NOT NULL,
    policy                TEXT    NOT NULL,
    visibility            TEXT    NOT NULL,
    corrects              TEXT,
    CONSTRAINT digest_requires_content_policy CHECK (
        content_digest_hex IS NULL
        OR json_extract(policy, '$.content_level') NOT IN ('exclude', 'metadata')
    ),
    CONSTRAINT digest_pair_complete CHECK (
        (content_digest_algo IS NULL) = (content_digest_hex IS NULL)
    )
);
CREATE INDEX IF NOT EXISTS observations_source_locator_idx ON observations (source_id, locator, seq DESC);
CREATE INDEX IF NOT EXISTS observations_digest_idx ON observations (content_digest_hex) WHERE content_digest_hex IS NOT NULL;

CREATE TRIGGER IF NOT EXISTS observations_no_update BEFORE UPDATE ON observations
BEGIN SELECT RAISE(ABORT, 'observations are append-only: UPDATE is not permitted'); END;
CREATE TRIGGER IF NOT EXISTS observations_no_delete BEFORE DELETE ON observations
BEGIN SELECT RAISE(ABORT, 'observations are append-only: DELETE is not permitted'); END;

CREATE TABLE IF NOT EXISTS current_files (
    source_id            TEXT    NOT NULL,
    locator              TEXT    NOT NULL,
    latest_seq           INTEGER NOT NULL,
    content_digest_hex   TEXT,
    size_bytes           INTEGER,
    observed_at          TEXT    NOT NULL,
    present              INTEGER NOT NULL DEFAULT 1,
    native_version_value TEXT    NOT NULL DEFAULT '',
    PRIMARY KEY (source_id, locator)
);
CREATE INDEX IF NOT EXISTS current_files_digest_idx ON current_files (content_digest_hex) WHERE content_digest_hex IS NOT NULL;
CREATE INDEX IF NOT EXISTS current_files_observed_idx ON current_files (observed_at DESC);

CREATE TABLE IF NOT EXISTS projector_state (
    projector  TEXT PRIMARY KEY,
    last_seq   INTEGER NOT NULL DEFAULT 0,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS source_cursors (
    source_id  TEXT PRIMARY KEY,
    cursor     TEXT NOT NULL,
    updated_at TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS identity_policies (
    version    TEXT PRIMARY KEY,
    profile    TEXT NOT NULL,
    scope      TEXT NOT NULL DEFAULT '',
    active     INTEGER NOT NULL DEFAULT 0,
    created_at TEXT NOT NULL DEFAULT ''
);
CREATE UNIQUE INDEX IF NOT EXISTS identity_policies_one_active ON identity_policies (active) WHERE active;

CREATE TABLE IF NOT EXISTS artifacts (
    identity_policy_version TEXT NOT NULL REFERENCES identity_policies(version) ON DELETE CASCADE,
    artifact_id  TEXT NOT NULL,
    source_id    TEXT NOT NULL,
    grouping_key TEXT NOT NULL,
    member_count INTEGER NOT NULL,
    confidence   REAL NOT NULL,
    PRIMARY KEY (identity_policy_version, artifact_id)
);

CREATE TABLE IF NOT EXISTS artifact_members (
    identity_policy_version TEXT NOT NULL,
    artifact_id   TEXT NOT NULL,
    source_id     TEXT NOT NULL,
    locator       TEXT NOT NULL,
    latest_seq    INTEGER NOT NULL,
    version_label TEXT,
    is_current    INTEGER NOT NULL,
    rule          TEXT NOT NULL,
    confidence    REAL NOT NULL,
    explanation   TEXT NOT NULL,
    PRIMARY KEY (identity_policy_version, artifact_id, source_id, locator),
    FOREIGN KEY (identity_policy_version, artifact_id)
        REFERENCES artifacts(identity_policy_version, artifact_id) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS artifact_members_locator_idx ON artifact_members (source_id, locator);

CREATE TABLE IF NOT EXISTS relations (
    identity_policy_version TEXT REFERENCES identity_policies(version) ON DELETE CASCADE,
    relation_id  TEXT PRIMARY KEY,
    type         TEXT NOT NULL,
    from_source  TEXT NOT NULL,
    from_locator TEXT NOT NULL,
    to_source    TEXT NOT NULL,
    to_locator   TEXT NOT NULL,
    precedence   TEXT NOT NULL,
    actor_kind   TEXT NOT NULL,
    actor_id     TEXT NOT NULL,
    evidence     TEXT NOT NULL,
    confidence   REAL NOT NULL,
    explanation  TEXT NOT NULL,
    asserted_at  TEXT NOT NULL,
    CONSTRAINT relation_cites_evidence CHECK (json_array_length(evidence) >= 1),
    CONSTRAINT machine_supersedes_needs_confidence CHECK (
        type <> 'supersedes'
        OR precedence <> 'gyst_suggestion'
        OR (confidence >= 0.8 AND length(explanation) > 0)
    )
);
CREATE INDEX IF NOT EXISTS relations_from_idx ON relations (from_source, from_locator);
CREATE INDEX IF NOT EXISTS relations_to_idx   ON relations (to_source, to_locator);
CREATE INDEX IF NOT EXISTS relations_type_idx ON relations (identity_policy_version, type);

CREATE TABLE IF NOT EXISTS sources (
    source_id           TEXT PRIMARY KEY,
    kind                TEXT NOT NULL,
    root                TEXT NOT NULL,
    first_seen          TEXT NOT NULL,
    last_seen           TEXT NOT NULL,
    location_kind       TEXT NOT NULL DEFAULT 'unknown',
    location_provider   TEXT NOT NULL DEFAULT '',
    location_mount      TEXT NOT NULL DEFAULT '',
    location_evidence   TEXT NOT NULL DEFAULT '',
    location_confidence REAL NOT NULL DEFAULT 0,
    cadence_seconds     INTEGER NOT NULL DEFAULT 0
);

CREATE TABLE IF NOT EXISTS commits (
    source_id      TEXT NOT NULL,
    oid            TEXT NOT NULL,
    seq            INTEGER NOT NULL,
    observation_id TEXT NOT NULL,
    author         TEXT NOT NULL,
    message        TEXT NOT NULL,
    authored_at    TEXT NOT NULL,
    parents        TEXT NOT NULL DEFAULT '[]',
    PRIMARY KEY (source_id, oid)
);
CREATE INDEX IF NOT EXISTS commits_authored_idx ON commits (authored_at DESC);

CREATE TABLE IF NOT EXISTS commit_files (
    source_id TEXT NOT NULL,
    oid       TEXT NOT NULL,
    locator   TEXT NOT NULL,
    PRIMARY KEY (source_id, oid, locator),
    FOREIGN KEY (source_id, oid) REFERENCES commits(source_id, oid) ON DELETE CASCADE
);
CREATE INDEX IF NOT EXISTS commit_files_locator_idx ON commit_files (source_id, locator);

CREATE TABLE IF NOT EXISTS scan_passes (
    pass_id         TEXT PRIMARY KEY,
    source_id       TEXT NOT NULL,
    connector       TEXT NOT NULL,
    started_at      TEXT NOT NULL,
    finished_at     TEXT,
    status          TEXT NOT NULL CHECK (status IN ('running','complete','partial','interrupted','unavailable')),
    detail          TEXT NOT NULL DEFAULT '',
    resumed         INTEGER NOT NULL DEFAULT 0,
    scanned         INTEGER NOT NULL DEFAULT 0,
    unchanged       INTEGER NOT NULL DEFAULT 0,
    skipped         INTEGER NOT NULL DEFAULT 0,
    ignored         INTEGER NOT NULL DEFAULT 0,
    unstable        INTEGER NOT NULL DEFAULT 0,
    bytes           INTEGER NOT NULL DEFAULT 0,
    hashed_bytes    INTEGER NOT NULL DEFAULT 0,
    appended        INTEGER NOT NULL DEFAULT 0,
    absence_checked INTEGER NOT NULL DEFAULT 0,
    absence_reason  TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS scan_passes_source_started_idx ON scan_passes (source_id, started_at DESC);

CREATE TABLE IF NOT EXISTS projects (
    project_id  TEXT PRIMARY KEY,
    name        TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    basis       TEXT NOT NULL CHECK (basis IN ('explicit','manifest','native-marker','organization-rule','suggestion')),
    source_id   TEXT NOT NULL,
    locator     TEXT NOT NULL,
    evidence    TEXT NOT NULL,
    confidence  REAL NOT NULL,
    explanation TEXT NOT NULL
);

CREATE TABLE IF NOT EXISTS project_members (
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    source_id  TEXT NOT NULL,
    pattern    TEXT NOT NULL,
    basis      TEXT NOT NULL,
    confidence REAL NOT NULL,
    evidence   TEXT NOT NULL,
    PRIMARY KEY (project_id, source_id, pattern)
);

CREATE TABLE IF NOT EXISTS file_projects (
    source_id  TEXT NOT NULL,
    locator    TEXT NOT NULL,
    project_id TEXT NOT NULL REFERENCES projects(project_id) ON DELETE CASCADE,
    basis      TEXT NOT NULL,
    confidence REAL NOT NULL,
    pattern    TEXT NOT NULL,
    PRIMARY KEY (source_id, locator, project_id)
);
CREATE INDEX IF NOT EXISTS file_projects_project_idx ON file_projects (project_id);

CREATE TABLE IF NOT EXISTS findings (
    finding_id   TEXT PRIMARY KEY,
    rule_id      TEXT NOT NULL,
    rule_version TEXT NOT NULL,
    severity     TEXT NOT NULL CHECK (severity IN ('info','low','medium','high')),
    status       TEXT NOT NULL CHECK (status IN ('open','acknowledged','waived','resolved')),
    subjects     TEXT NOT NULL,
    evidence     TEXT NOT NULL,
    detected_at  TEXT NOT NULL,
    last_seen_at TEXT NOT NULL,
    resolved_at  TEXT,
    confidence   REAL NOT NULL,
    summary      TEXT NOT NULL,
    remediation  TEXT,
    waiver       TEXT,
    disposed_by  TEXT NOT NULL DEFAULT '',
    disposed_at  TEXT,
    CONSTRAINT waived_requires_waiver CHECK (status <> 'waived' OR waiver IS NOT NULL)
);
CREATE INDEX IF NOT EXISTS findings_status_idx ON findings (status, severity);

CREATE TABLE IF NOT EXISTS assertions (
    assertion_id   TEXT PRIMARY KEY,
    kind           TEXT NOT NULL CHECK (kind IN ('authority','not-authority')),
    source_id      TEXT NOT NULL,
    locator        TEXT NOT NULL,
    actor_kind     TEXT NOT NULL CHECK (actor_kind = 'user'),
    actor_id       TEXT NOT NULL,
    reason         TEXT NOT NULL,
    evidence       TEXT NOT NULL,
    asserted_at    TEXT NOT NULL,
    retracted_at   TEXT,
    retracted_by   TEXT,
    retract_reason TEXT,
    CONSTRAINT retraction_complete CHECK (
        (retracted_at IS NULL) = (retracted_by IS NULL)
        AND (retracted_at IS NULL) = (retract_reason IS NULL))
);
CREATE INDEX IF NOT EXISTS assertions_subject_idx ON assertions (source_id, locator);

CREATE TRIGGER IF NOT EXISTS assertions_no_delete BEFORE DELETE ON assertions
BEGIN SELECT RAISE(ABORT, 'assertions are never deleted: retract instead'); END;
CREATE TRIGGER IF NOT EXISTS assertions_no_edit BEFORE UPDATE ON assertions
WHEN OLD.retracted_at IS NOT NULL
  OR NEW.kind <> OLD.kind OR NEW.source_id <> OLD.source_id OR NEW.locator <> OLD.locator
  OR NEW.actor_kind <> OLD.actor_kind OR NEW.actor_id <> OLD.actor_id
  OR NEW.reason <> OLD.reason OR NEW.evidence <> OLD.evidence OR NEW.asserted_at <> OLD.asserted_at
BEGIN SELECT RAISE(ABORT, 'assertions are never edited: only a retraction may be recorded'); END;

CREATE TABLE IF NOT EXISTS file_authority (
    source_id         TEXT NOT NULL,
    locator           TEXT NOT NULL,
    state             TEXT NOT NULL CHECK (state IN ('declared','likely','multiple','none')),
    basis             TEXT NOT NULL,
    authority_source  TEXT,
    authority_locator TEXT,
    confidence        REAL NOT NULL,
    evidence          TEXT NOT NULL,
    assertion_id      TEXT,
    explanation       TEXT NOT NULL,
    PRIMARY KEY (source_id, locator)
);
