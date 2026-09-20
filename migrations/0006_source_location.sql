-- Where a source root physically lives.
--
-- A local folder, a network share, and a cloud-synced folder are governed by
-- different rules in the architecture: shares get scheduled passes and may be
-- unavailable, synced folders may hold placeholder files and must never hold
-- Git metadata, local folders are fast and complete. The kind is a policy
-- input, so it is recorded on the source when it is registered and shown next
-- to every root. The evidence column says how it was decided; the confidence
-- is honest about heuristics (a marker file) versus declarations (the sync
-- client's own configuration).

BEGIN;

ALTER TABLE sources
    ADD COLUMN IF NOT EXISTS location_kind       TEXT NOT NULL DEFAULT 'unknown',
    ADD COLUMN IF NOT EXISTS location_provider   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS location_mount      TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS location_evidence   TEXT NOT NULL DEFAULT '',
    ADD COLUMN IF NOT EXISTS location_confidence REAL NOT NULL DEFAULT 0;

COMMIT;
