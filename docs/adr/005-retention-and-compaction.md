# ADR 005: Retention and compaction of the observation log

Status: accepted, 2026-09-20, by Daniel.

## Context

The observation log is append-only and the database enforces it: no row
is ever updated or deleted, on either engine. Everything else is a
projection rebuilt from the log. That is the product's central
guarantee, and three of the constitutional constraints rest on it: every
derived claim keeps its evidence, source files remain authoritative, and
a user's exported records remain usable if Gyst disappears.

It also means nothing is ever removed. `decisions.md` has carried
"compaction, replay, and retention rules" as an unresolved question since
the walking skeleton. This record proposes an answer, from measurement.

### What accumulates, measured

A hundred thousand files scanned on PostgreSQL, then three rounds of
churn, each modifying ten percent of the files and adding and deleting one
percent, with a rescan after each:

| Table | After first scan | After three churn rounds | Per row |
|---|---:|---:|---:|
| `observations` | 100,029 rows, 98 MB | 136,029 rows, 134 MB | about 1 KB with indexes |
| `current_files` | 100,000 rows, 41 MB | 103,000 rows, 44 MB | about 420 B; absent rows stay |
| `file_authority` | 100,000 rows, 48 MB | 100,000 rows, 66 MB | rebuilt each pass; the growth is dead tuples, not rows |
| `file_projects` | 14,500 rows, 6 MB | 14,497 rows, 8 MB | same |
| `findings` | 1 row | 4 rows | one resolved per round, see below |
| `scan_passes` | 2 rows | 5 rows | about 200 B, one per pass |
| Whole database | 201 MB | 261 MB | |

Each churn round appended 12,000 observations: 10,000 changed
fingerprints, 1,000 arrivals, 1,000 tombstones. Twelve megabytes a round.

### What drives growth

Change, not scanning. A rescan of an unchanged tree appends nothing,
because unchanged files are filtered against known state before an
observation is built and a re-read manifest derives the same id. Scanning
hourly instead of daily costs one pass row an hour and nothing in the log.
So the log's size is a function of how much the sources change, which is
the history the product promises to keep. A team changing ten thousand
files a day writes about ten megabytes a day, three and a half gigabytes a
year. A team changing a hundred a day writes forty megabytes a year.

The rebuilt projections cost as much as the log at baseline and do not
grow with history; on PostgreSQL they leave dead tuples for autovacuum,
which is ordinary and not a retention question.

### What must never be removed

Some observations are load-bearing in ways the log's shape does not show:

- Any observation cited as evidence by a relation, a finding, an
  assertion, or, when they exist, a release manifest. Removing one breaks
  the chain the product exists to keep.
- The latest observation of every locator. Replay rebuilds `current_files`
  from it; without it the projection is not reproducible.
- The first observation of every locator. Rename detection defines an
  arrival as a locator's first observation, and a compare-set relation's
  meaning depends on it.
- Every tombstone whose locator has not been observed again since.
- Every assertion, retracted or not. They are not in the log, but the same
  rule applies: a person's record is never removed.

What remains removable is an intermediate observation of a file that
changed several times and was never cited by anything: the middle
revisions of a file that was edited daily for a month and about which
nothing was concluded.

## Options

### A. Keep everything

The status quo. Cost is one kilobyte per change forever. On the
measurements above that is fine for years for most teams and becomes a
question at tens of millions of changes, where the correlated subqueries
in the projections slow before disk does. It keeps every promise by
construction.

### B. Delete uncited intermediates past a horizon

A privileged path removes observations that are older than a horizon and
not in the protected set. Replay still reproduces the projection, since
the latest per locator survives. But the log is no longer the complete
history it claims to be, `gyst explain` shows a gap it cannot account for,
and a record that was exported before compaction and re-imported after it
disagrees with the live log. The append-only trigger would have to be
bypassed, and a bypass that exists will be used.

### C. Archive, then compact

Removable observations are first written to an archive bundle: an ordered
file of observation envelopes, in the same JSON the schemas define, with a
manifest naming the seq range, the count, and a digest. The bundle is
verified by reading it back. Only then are those observations removed
from the live log, and a `compactions` record keeps the bundle's path,
digest, seq range, and time. `gyst explain` shows "N earlier observations
archived on <date>, bundle <digest>", and a future `gyst import` can bring
a bundle back. Nothing is lost; cold history moves out of the working
set. This is the transfer-bundle format the roadmap already owes, used
for a second purpose.

### D. Bookkeeping only

Leave the log alone and bound only what is not evidence: keep the last
thirty passes per source plus the first, and delete resolved findings
older than a horizon unless they carry a waiver, which is an audit
record. This changes nothing about provenance and almost nothing about
size; it is hygiene.

## Decision (proposed)

D now, C when someone needs it, never B, and A as the default until then.

Concretely:

1. **Bookkeeping retention, on by default.** Passes: keep the newest
   thirty per source and the first ever; findings: delete resolved rows
   older than ninety days that have no waiver. Both run at the end of a
   scan. Neither touches the log. *Done 2026-09-20; see
   `docs/day-20-notes.md`.*
2. **Compaction is manual, per store, and archives first.**
   `gyst compact --older-than 180d --archive <dir>` selects removable
   observations, writes the bundle, verifies it, removes them in one
   transaction, and records the compaction. It refuses to run without an
   archive path. It never runs on a schedule.
3. **The append-only guarantee stays enforced by the database.** The
   triggers gain one exception: a delete is permitted while a
   `compaction_gate` row is open, and only the compaction method opens it,
   inside its own transaction, and closes it before commit. This is the
   same spelling on both engines and leaves no bypass outside the store.
4. **Retention is a per-source policy field**, like cadence, with the
   horizon in days and zero meaning never. When `.gyst/policy.yaml`
   exists it will carry it; until then a `--retention` flag on scan sets
   it, and compaction honours the shortest horizon among the sources an
   observation belongs to.

## Acceptance

- After compaction, `gyst verify` reproduces the projection exactly, and
  the projection fingerprint equals the one before compaction.
- Every evidence id cited by any relation, finding, or assertion still
  resolves in the live log.
- Every locator keeps its first and latest observation; every unresolved
  tombstone survives.
- The archive bundle validates against the observation schema, and
  re-importing it into an empty store followed by replay produces a
  projection whose fingerprint matches the pre-compaction one.
- The trigger still refuses a delete outside the gate, on both engines,
  and the store suite proves it.
- Compaction of the benchmark database after three churn rounds removes
  the 30,000 intermediate fingerprints and nothing else, and the measured
  time is recorded.

## Consequences

- One new table, `compactions`, and one new column on `sources`. One new
  command. The transfer-bundle writer that ADR 012 on the list needs gets
  built here first, in its read-only half.
- `gyst explain` gains a line for archived history, and the report's
  `report` block gains a count of compactions and the horizon in force.
- The findings package must give large duplicate groups a stable id
  before finding retention is safe; see below.
- Estimated effort: one session for D, two for C including the bundle
  format and its tests.

## Found while measuring

The duplicate-content finding for a large group got a new id every churn
round. Its id derives from its subjects, and the subjects are the first
twenty-five files of the group, which change as files are modified. An
acknowledgement or waiver on such a finding would not survive the next
scan. The id should derive from the rule and the digest for a content
group, not from the sampled subjects. That is a defect in the day-15
bound, independent of this record, and is the first thing to fix.
*Fixed the same day: the id is now the rule plus the content digest.*

## Not decided here

- The bundle format's signature and trust bootstrap, which belong to the
  air-gap transfer record.
- Whether the archive can be read directly by `gyst explain` rather than
  only imported.
- Retention of the report documents themselves, which are files the
  user owns.
