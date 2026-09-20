-- Findings: a rule and the evidence it matched, kept across rebuilds.
--
-- Most projections are dropped and rebuilt. Findings cannot be, because a
-- person acts on them: acknowledging one, or waiving it with a reason that
-- has to survive the next scan. So a finding has a stable id derived from
-- its rule and subjects, detection upserts it, and a disposition set by a
-- person is never overwritten by the machine. A finding the rules no longer
-- produce is resolved, not deleted, so the record of what was seen remains.
--
-- Only a person may waive. The schema says so and the CLI enforces the
-- actor kind; nothing in this table can be waived by a rule.

BEGIN;

CREATE TABLE IF NOT EXISTS findings (
    finding_id    TEXT PRIMARY KEY,
    rule_id       TEXT NOT NULL,
    rule_version  TEXT NOT NULL,
    severity      TEXT NOT NULL CHECK (severity IN ('info','low','medium','high')),
    status        TEXT NOT NULL CHECK (status IN ('open','acknowledged','waived','resolved')),
    subjects      JSONB NOT NULL,
    evidence      TEXT[] NOT NULL,
    detected_at   TIMESTAMPTZ NOT NULL,   -- first detection; kept on re-detection
    last_seen_at  TIMESTAMPTZ NOT NULL,   -- most recent detection
    resolved_at   TIMESTAMPTZ,
    confidence    REAL NOT NULL,
    summary       TEXT NOT NULL,
    remediation   JSONB,
    waiver        JSONB,
    -- Who last changed the status by hand, so a reopened finding can say
    -- "acknowledged by X on Y, detected again".
    disposed_by   TEXT NOT NULL DEFAULT '',
    disposed_at   TIMESTAMPTZ,
    CONSTRAINT waived_requires_waiver CHECK (status <> 'waived' OR waiver IS NOT NULL)
);

CREATE INDEX IF NOT EXISTS findings_status_idx ON findings (status, severity);
CREATE INDEX IF NOT EXISTS findings_subjects_idx ON findings USING GIN (subjects jsonb_path_ops);

-- How often a source is expected to be scanned. Zero means the default for
-- its location kind. Stale is "older than this", not "older than a day".
ALTER TABLE sources ADD COLUMN IF NOT EXISTS cadence_seconds INT NOT NULL DEFAULT 0;

COMMIT;
