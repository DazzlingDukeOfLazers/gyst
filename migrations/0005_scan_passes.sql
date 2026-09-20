-- Scan passes: one row per attempt to observe a source.
--
-- Until now a pass existed only implicitly, as the set of observations sharing
-- one observed_at. That answered "what was seen" but not "how much of the source
-- was looked at", and the two are different questions: a pass that stopped at
-- --max-files, resumed from a cursor, hit an unreadable directory, or found the
-- root unmounted has produced true observations and incomplete coverage. A
-- report that shows "seen 3 days ago" without "and the scan did not finish" is
-- telling half the truth.
--
-- This table is bookkeeping, not evidence. It is updated in place (a pass
-- begins, then finishes) and may be rebuilt or dropped without touching the
-- log. started_at equals the observed_at of every observation the pass
-- produced, which is the join. An explicit pass_id column on observations is
-- the sturdier design (see docs/day-5-notes.md) but changes the envelope
-- schema, and that is a contract change to make deliberately, not in passing.

BEGIN;

CREATE TABLE IF NOT EXISTS scan_passes (
    pass_id         TEXT        PRIMARY KEY,
    source_id       TEXT        NOT NULL,
    connector       TEXT        NOT NULL,
    started_at      TIMESTAMPTZ NOT NULL,
    finished_at     TIMESTAMPTZ,

    -- running      begun, no result recorded yet
    -- complete     the whole source was visited and nothing was unreadable
    -- partial      some of the source was deliberately or unavoidably not
    --              visited: --max-files, a resumed cursor, unreadable entries
    -- interrupted  began and never reported a result; the process died or a
    --              later pass on the same source began first
    -- unavailable  the root could not be opened at all
    status          TEXT        NOT NULL
                    CHECK (status IN ('running','complete','partial','interrupted','unavailable')),
    detail          TEXT        NOT NULL DEFAULT '',
    resumed         BOOLEAN     NOT NULL DEFAULT FALSE,

    scanned         INT         NOT NULL DEFAULT 0,
    unchanged       INT         NOT NULL DEFAULT 0,
    skipped         INT         NOT NULL DEFAULT 0,
    ignored         INT         NOT NULL DEFAULT 0,
    unstable        INT         NOT NULL DEFAULT 0,
    bytes           BIGINT      NOT NULL DEFAULT 0,
    hashed_bytes    BIGINT      NOT NULL DEFAULT 0,
    appended        INT         NOT NULL DEFAULT 0,

    -- Whether absence was checked is a property of the pass, and the reason it
    -- was not is what a person needs to see next to a stale file.
    absence_checked BOOLEAN     NOT NULL DEFAULT FALSE,
    absence_reason  TEXT        NOT NULL DEFAULT ''
);

CREATE INDEX IF NOT EXISTS scan_passes_source_started_idx
    ON scan_passes (source_id, started_at DESC);

COMMIT;
