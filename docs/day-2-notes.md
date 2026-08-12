# Day 2 notes: walking skeleton

Scan a root into an append-only log, project it, query the result. Both exit
criteria from the plan hold, and one design bug surfaced and was fixed.

## Exit criteria

| Criterion | Result |
|---|---|
| Restart/replay produces equivalent projections | `gyst verify` replays 50,019 observations from seq 0 and reproduces the projection fingerprint exactly, in 2.3 s. |
| A second scan over an unchanged tree emits no new observations | 0 appended, and no file is read at all. |

## The bug worth recording

The first design derived an observation id from observed state alone: source,
locator, native version, claim type, extractor. Re-scanning an unchanged file
re-derived the same id, collided on the unique index, and appended nothing. That
looked like an elegant idempotency mechanism.

It silently loses a revert. Edit a file, then restore it to a byte-identical
earlier state with its original mtime — which `git checkout` and a backup
restore both reproduce exactly — and the scanner re-derives the *original* id.
The insert collides, nothing is appended, and the projection goes on asserting
the intermediate content permanently. Observed directly: the file on disk hashed
to `072b5741…` while `current_files` insisted on `e9aea4e2…`.

Two changes fix it, and the second is the real one:

1. `DeriveID` takes the seq of the observation being superseded. Same bytes
   after a different history is a different observation.
2. Unchanged files are filtered against known projection state *before* an
   observation is built. An id collision is now a genuine duplicate — two agents
   scanning concurrently, or a retried batch — rather than the mechanism.

`TestDeriveIDDistinguishesRevertToEarlierState` locks this in.

The general lesson is worth carrying into the Git and cloud connectors: an
identifier derived from state answers "have I seen this state?", which is not
the question. The question is "is this different from what I currently believe?"

## What the fix bought

Comparing against known state before reading means an incremental scan never
hashes a settled tree.

| Scan | Files | Hashed | Wall clock |
|---|---|---|---|
| Full, cold | 50,000 | 145.8 MB | 5.25 s |
| Incremental, nothing changed | 50,000 | 0 B | 273 ms |
| Incremental, 3 files changed | 50,000 | 56.4 KB | 790 ms |

Hashing throughput on the full pass was 27.8 MB/s, and the directory walk alone
was 2.96 s of the 5.25 s — so the walk, not the hash, is the next thing to
attack if this needs to be faster.

These numbers are from a synthetic tree on one machine, and the Phase 0 exit
criterion asks for a *representative* dataset. Treat them as an order of
magnitude, not a measurement of anything real.

## Enforced rather than described

- `observations` rejects `UPDATE` and `DELETE` by trigger, and `TRUNCATE` by a
  separate statement-level trigger. The row-level trigger alone did not see
  `TRUNCATE`, which would have emptied the log without firing anything.
- A `CHECK` constraint rejects a content digest recorded under `exclude` or
  `metadata` policy, mirroring the JSON Schema rule.
- The projector's upsert carries `WHERE latest_seq < EXCLUDED.latest_seq`, so a
  replayed or out-of-order observation cannot regress the projection.

Consequence of the truncate guard: there is no in-place reset. Rebuilding a
development log means dropping the database.

## Known gaps

- **`seq` is sparse.** `BIGSERIAL` consumes a value even when `ON CONFLICT DO
  NOTHING` discards the row, so gaps appear. Ordering is unaffected, but nothing
  should treat `seq` as a dense count.
- **No tombstones.** A deleted file is never marked absent, because the scan only
  observes what exists. `artifact.absent` is handled by the projector and the
  schema but nothing emits it. Deletion detection needs the full-pass locator set
  diffed against known state.
- **Stability check is an approximation.** Files modified within 2 s are re-stat'ed
  before hashing; a slow writer that pauses longer can still be caught mid-write.
- **Unicode normalisation is untested across platforms.** `désign-notes.md`
  round-trips consistently on macOS, but NFC/NFD differences between macOS and
  Windows are exactly the case that would split one file into two, and that
  cannot be tested on one host.
- **Go structs and JSON Schemas are kept in step by hand.** Nothing validates
  connector output against `schemas/v0/` at build time.
- **`--resume` is untested against interruption.** The cursor is written, but no
  test kills a scan mid-pass and checks the resumed result.
