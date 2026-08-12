-- Day 2 walking skeleton: an append-only observation log and one projection
-- rebuilt from it.
--
-- The log is the only authority here. Every other table in this file can be
-- dropped and reconstructed by replaying observations in seq order, and the
-- day 2 exit criterion is exactly that.

BEGIN;

CREATE TABLE IF NOT EXISTS observations (
    -- Total order for replay. The projector's only ordering key: observed_at
    -- comes from the observer's clock and cannot be trusted for sequencing.
    seq                   BIGSERIAL PRIMARY KEY,

    -- Idempotency key. Derived deterministically from the observed state
    -- (see observe.DeriveID), so re-observing an unchanged file produces the
    -- same id and the insert collides instead of appending a duplicate.
    observation_id        TEXT        NOT NULL UNIQUE,

    schema_version        TEXT        NOT NULL,
    source_id             TEXT        NOT NULL,
    connector             TEXT        NOT NULL,
    connector_version     TEXT        NOT NULL,
    cursor                TEXT,

    observed_at           TIMESTAMPTZ NOT NULL,
    recorded_at           TIMESTAMPTZ NOT NULL DEFAULT now(),

    subject_kind          TEXT        NOT NULL,
    locator               TEXT        NOT NULL,
    native_version_scheme TEXT        NOT NULL,
    native_version_value  TEXT        NOT NULL,

    -- Null whenever effective policy forbade reading content. The schema
    -- rejects a digest under exclude/metadata policy; this mirrors it.
    content_digest_algo   TEXT,
    content_digest_hex    TEXT,
    size_bytes            BIGINT,

    claim_type            TEXT        NOT NULL,
    claim_payload         JSONB       NOT NULL,
    extractor             JSONB       NOT NULL,
    policy                JSONB       NOT NULL,
    visibility            JSONB       NOT NULL,

    -- A correction is a new observation citing the one it supersedes. The
    -- corrected row stays in the log.
    corrects              TEXT,

    CONSTRAINT digest_requires_content_policy CHECK (
        content_digest_hex IS NULL
        OR policy ->> 'content_level' NOT IN ('exclude', 'metadata')
    ),
    CONSTRAINT digest_pair_complete CHECK (
        (content_digest_algo IS NULL) = (content_digest_hex IS NULL)
    )
);

CREATE INDEX IF NOT EXISTS observations_source_locator_idx
    ON observations (source_id, locator, seq DESC);
CREATE INDEX IF NOT EXISTS observations_digest_idx
    ON observations (content_digest_hex) WHERE content_digest_hex IS NOT NULL;

-- Append-only is enforced here rather than trusted to application code. An
-- invariant that only holds while every caller behaves is not an invariant.
CREATE OR REPLACE FUNCTION observations_are_immutable() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION
        'observations are append-only: attempted % on observation_id=%',
        TG_OP, COALESCE(OLD.observation_id, NEW.observation_id);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS observations_no_mutation ON observations;
CREATE TRIGGER observations_no_mutation
    BEFORE UPDATE OR DELETE ON observations
    FOR EACH ROW EXECUTE FUNCTION observations_are_immutable();

-- A row-level trigger does not see TRUNCATE, which would empty the log without
-- firing anything above. Statement-level guard closes that path.
--
-- Consequence: there is no in-place reset. Rebuilding a development log means
-- dropping the database (see README). That is the intended cost of the claim.
CREATE OR REPLACE FUNCTION observations_reject_truncate() RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION 'observations are append-only: TRUNCATE is not permitted';
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS observations_no_truncate ON observations;
CREATE TRIGGER observations_no_truncate
    BEFORE TRUNCATE ON observations
    FOR EACH STATEMENT EXECUTE FUNCTION observations_reject_truncate();

-- ---------------------------------------------------------------------------
-- Projections. Everything below is derived and disposable.
-- ---------------------------------------------------------------------------

CREATE TABLE IF NOT EXISTS current_files (
    source_id           TEXT        NOT NULL,
    locator             TEXT        NOT NULL,
    latest_seq          BIGINT      NOT NULL,
    content_digest_hex  TEXT,
    size_bytes          BIGINT,
    observed_at         TIMESTAMPTZ NOT NULL,
    present             BOOLEAN     NOT NULL DEFAULT TRUE,
    PRIMARY KEY (source_id, locator)
);

CREATE INDEX IF NOT EXISTS current_files_digest_idx
    ON current_files (content_digest_hex) WHERE content_digest_hex IS NOT NULL;
CREATE INDEX IF NOT EXISTS current_files_observed_idx
    ON current_files (observed_at DESC);

-- How far each projector has consumed the log. Restart resumes from here;
-- setting it to 0 forces a full rebuild.
CREATE TABLE IF NOT EXISTS projector_state (
    projector    TEXT   PRIMARY KEY,
    last_seq     BIGINT NOT NULL DEFAULT 0,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- Connector cursors, so a scan is resumable and does not restart from zero.
CREATE TABLE IF NOT EXISTS source_cursors (
    source_id    TEXT PRIMARY KEY,
    cursor       TEXT NOT NULL,
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

COMMIT;
