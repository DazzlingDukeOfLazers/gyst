# Day 6 notes: scan passes

A pass existed only by implication: the set of observations sharing one
`observed_at`. That answers "what was seen" and says nothing about how much of
the source was looked at, and the two questions have different answers more
often than not. A scan stopped by `--max-files`, resumed from a cursor, blocked
by an unreadable directory, or pointed at an unmounted share has produced true
observations and incomplete coverage. "Seen three days ago" without "and the
scan did not finish" is half the truth, and the half that is missing is the
half a person needs when deciding whether a file is stale.

## What a pass records

`scan_passes` holds one row per attempt on a source: when it began, when it
ended, its coverage, the counts, and whether absence was checked. The row is
begun before the walk and finished after it, so a pass that dies leaves a row
saying `running`, and the next pass on the same source concludes the earlier
one was `interrupted`. Two genuinely concurrent scanners would misreport each
other; the log is safe under concurrency and this table is bookkeeping, so
that is accepted rather than guarded.

Coverage has four values:

| Status | Meaning |
|---|---|
| complete | The whole source was visited and nothing was unreadable. |
| partial | Some of it was not: `--max-files`, a resumed cursor, or unreadable entries. The detail says which. |
| interrupted | Began and never reported a result. |
| unavailable | The root could not be opened at all. |

The scan prints its coverage on every run, and `gyst status` now shows the
latest pass for every registered source instead of raw cursors. A source with
no pass is shown as `never scanned`: that is a state the report needs, not a
missing row.

## One definition of "looked everywhere"

Tombstones already refused to fire unless the pass was complete, unresumed,
and free of read errors. Those three conditions are now `Result.Coverage`,
and `Tombstones` consults it instead of restating them. A test asserts the
two agree for every combination, because the failure mode of two copies is
silent: a pass recorded as complete that refused to tombstone, or worse, one
recorded as partial that did.

## An unmounted share is not an empty folder

Before today a missing root walked as an empty tree: one skipped entry, zero
files, and a "complete" pass. Tombstoning was suppressed only because the
skipped count happened to be non-zero. `Discover` now stats the root first and
returns `UnavailableError`, and the scan records the pass as unavailable with
nothing else moved: no observations, no cursor, no tombstones. The exit code
is zero. Unavailable is a true statement about the source, not a failure of
the tool.

## The clock

The pass clock moved out of `Discover` and into the caller. The scan takes one
reading, records it as the pass's `started_at`, and hands it to the connector
so every observation carries the same instant. The pass row and the log agree
about when the pass happened by construction rather than by coincidence, and
a test can now hand in a fixed clock.

## What this does not do

- `observed_at` is still the key that groups a pass's observations, joined to
  `started_at`. An explicit `pass_id` on the observation is the sturdier
  design, but it changes the envelope schema, and that is a contract change
  to make on purpose with the examples updated in the same change.
- Expected scan cadence per source, and therefore "stale" as distinct from
  "old", is not recorded yet. The design wants both; this adds the age and the
  coverage, which are the two inputs.
- Git passes record complete or partial only. Git has no unreadable-directory
  case, and an unavailable repository is still a plain error.
