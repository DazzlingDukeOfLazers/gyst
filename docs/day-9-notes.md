# Day 9 notes: findings

The fourth of the product's questions is "what needs attention?" Until today
the answer lived in the reader's head: duplicates could be found by querying
digests, ambiguity sat in compare-set relations, a stale share showed only
as an old timestamp. A finding is the same evidence, named, cited, and kept
until a person has dealt with it.

## Five rules

| Rule | Severity | Fires when |
|---|---|---|
| `hygiene.duplicate-content` | low | Two or more present files share a digest, in one source or across several. Empty files and files never read are excluded: every empty file shares one digest, and a file policy kept Gyst from reading has nothing to compare. |
| `hygiene.ambiguous-origin` | medium | A file is gone and two or more present files match its content. The rename detector refused to choose; this asks a person to. |
| `project.manifest-invalid` | medium | A present `.gyst/project.yaml` does not parse. The project it declares does not exist until it is fixed. |
| `source.stale` | medium | The latest pass on a source is older than its cadence. |
| `source.unavailable` | medium | The latest pass could not open the root. |
| `source.interrupted` | low | The latest pass began and never reported. |

Cadence is per source, set with `--cadence 7d` on a scan, and defaults by
location kind: a week for a network share, a day for everything else. Stale
means "older than expected for this source", not "older than a day". Never
scanned is a state the status table shows, not a finding.

Every rule is a pure function over loaded inputs, so each one is tested
without a database.

## Evidence is observations, and that shapes what can be said

The schema requires every finding to cite at least one observation. A pass
is bookkeeping, not an observation, so a finding about a source cites the
most recent observation from that source: the rule fires because of what is
known, and that is the last thing known. The consequence is that a source
that was never successfully observed cannot be the subject of a finding at
all. There is nothing to cite. It appears in `gyst status` as unavailable or
never scanned, and that is where it belongs.

The source subject's native version uses the `none` scheme, because a root
has no version of its own. The first draft invented a scheme and the JSON
output failed validation, which is exactly what the validator is for.

## Findings persist; projections do not

Everything else derived from the log is dropped and rebuilt. Findings
cannot be, because a person acts on them. So a finding's id is derived from
its rule and its subjects, not from the evidence or the time, and detection
upserts: a new finding opens, a re-detected one keeps whatever a person
decided, and one no longer produced is resolved with a timestamp rather
than deleted. A re-detected resolved finding reopens. A waiver with an
expiry reopens the finding when the expiry passes.

The upsert's first draft read the prior status with a scalar subquery in
`RETURNING`, which sees the row after the write. A CTE that reads before the
insert gives the old status, and NULL for a new row, which is also the exact
reopened count.

Only a person may waive. The schema says so, the function refuses any other
actor kind before touching the store, and the table has a check constraint
that a waived row carries a waiver. Acknowledging and waiving both record
who and when.

## Output is the contract

`gyst findings --json` emits the v0 finding schema directly, and the live
output was validated against it with the same registry the example
validator uses: 27 findings, 0 invalid. This is the shape the report will
consume.

`gyst explain <file>` now ends with the open findings that name the file.

## What this does not do

- No `content.placeholder` finding. Placeholders are visible on the
  observation, but a per-source summary finding with partial subjects felt
  worse than none. Open question for the design: is "N files not present
  locally" a finding or a source property?
- No generated-file, stale-copy, or authority findings. Those need the
  identity groups and generation runs joined in.
- Rename detection's "an arrival is a locator's first observation ever"
  rule means a file deleted and recreated at a path seen before is never an
  arrival, so an ambiguity involving it is not detected. This is a day-5
  limit, not a findings one, and it showed up in testing against a
  database with leftover state.
- Findings for a source that has no observations cannot exist. See above.
