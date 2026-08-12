-- The projection could not answer "is what I just saw different from what I
-- already know?" without the source's own version, so an unchanged-looking
-- rescan could not be distinguished from a revert to an earlier state.
--
-- Concretely: a file edited and then restored to a byte-identical earlier state
-- (same mtime, same size) re-derived the observation id it had the first time,
-- collided on the unique index, appended nothing, and left the projection
-- asserting the intermediate content forever.
--
-- This column is what lets a scan compare observed state against known state
-- directly rather than inferring it from an id collision. It also lets the
-- connector skip hashing a file whose native version is unchanged.
--
-- current_files is a projection, so this needs no backfill: reset the projector
-- cursor and replay.

BEGIN;

ALTER TABLE current_files
    ADD COLUMN IF NOT EXISTS native_version_value TEXT NOT NULL DEFAULT '';

COMMIT;
