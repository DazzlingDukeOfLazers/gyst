-- Identity grouping and derived relations.
--
-- Every table here is keyed by identity_policy_version. That is what makes a
-- profile switch reversible: activating a new profile writes a new policy
-- version alongside the old one rather than mutating it, and the observation
-- log is never touched at all.
--
-- Dropping every row in this file and rebuilding from the log must produce the
-- same result, which is what `gyst identity verify` checks.

BEGIN;

CREATE TABLE IF NOT EXISTS identity_policies (
    version     TEXT PRIMARY KEY,
    profile     TEXT        NOT NULL,
    scope       TEXT        NOT NULL DEFAULT '',
    active      BOOLEAN     NOT NULL DEFAULT FALSE,
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Only one policy is active at a time, but superseded ones are retained: a
-- released manifest pins the interpretation in force when it was approved, and
-- that interpretation has to remain resolvable afterwards.
CREATE UNIQUE INDEX IF NOT EXISTS identity_policies_one_active
    ON identity_policies ((true)) WHERE active;

CREATE TABLE IF NOT EXISTS artifacts (
    identity_policy_version TEXT   NOT NULL REFERENCES identity_policies(version) ON DELETE CASCADE,
    artifact_id             TEXT   NOT NULL,
    source_id               TEXT   NOT NULL,
    grouping_key            TEXT   NOT NULL,
    member_count            INT    NOT NULL,
    confidence              REAL   NOT NULL,
    PRIMARY KEY (identity_policy_version, artifact_id)
);

CREATE TABLE IF NOT EXISTS artifact_members (
    identity_policy_version TEXT    NOT NULL,
    artifact_id             TEXT    NOT NULL,
    source_id               TEXT    NOT NULL,
    locator                 TEXT    NOT NULL,
    latest_seq              BIGINT  NOT NULL,
    version_label           TEXT,
    is_current              BOOLEAN NOT NULL,
    rule                    TEXT    NOT NULL,
    confidence              REAL    NOT NULL,
    explanation             TEXT    NOT NULL,
    PRIMARY KEY (identity_policy_version, artifact_id, source_id, locator),
    FOREIGN KEY (identity_policy_version, artifact_id)
        REFERENCES artifacts(identity_policy_version, artifact_id) ON DELETE CASCADE
);

CREATE INDEX IF NOT EXISTS artifact_members_locator_idx
    ON artifact_members (source_id, locator);

CREATE TABLE IF NOT EXISTS relations (
    identity_policy_version TEXT        NOT NULL REFERENCES identity_policies(version) ON DELETE CASCADE,
    relation_id             TEXT        NOT NULL,
    type                    TEXT        NOT NULL,
    from_source             TEXT        NOT NULL,
    from_locator            TEXT        NOT NULL,
    to_source               TEXT        NOT NULL,
    to_locator              TEXT        NOT NULL,
    precedence              TEXT        NOT NULL,
    actor_kind              TEXT        NOT NULL,
    actor_id                TEXT        NOT NULL,
    evidence                TEXT[]      NOT NULL,
    confidence              REAL        NOT NULL,
    explanation             TEXT        NOT NULL,
    asserted_at             TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (identity_policy_version, relation_id),

    -- Mirrors the JSON Schema rule. A derived record that cannot cite evidence
    -- must not exist, in the database as well as on the wire.
    CONSTRAINT relation_cites_evidence
        CHECK (array_length(evidence, 1) >= 1),

    -- Mirrors the supersedes threshold. Gyst must never conclude that one file
    -- supersedes another from a numeric suffix alone, so a machine-suggested
    -- supersedes needs both an explanation and confidence at or above 0.8.
    -- Enforced here as well as in Go because the rule is a product commitment,
    -- not an implementation detail of one code path.
    CONSTRAINT machine_supersedes_needs_confidence CHECK (
        type <> 'supersedes'
        OR precedence <> 'gyst_suggestion'
        OR (confidence >= 0.8 AND length(explanation) > 0)
    )
);

CREATE INDEX IF NOT EXISTS relations_from_idx ON relations (from_source, from_locator);
CREATE INDEX IF NOT EXISTS relations_to_idx   ON relations (to_source, to_locator);
CREATE INDEX IF NOT EXISTS relations_type_idx ON relations (identity_policy_version, type);

COMMIT;
