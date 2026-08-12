# Day 4 notes: tombstones and commit/file reconciliation

The two gaps carried out of day 3. Deletion detection closes "what changed?" in
the direction it could not answer, and reconciliation joins the two views of a
file that both existed and never met.

## Tombstones

A scan observes what exists. Deletion is the one claim it makes about something
it did not see, so it is the one claim that has to justify itself.

Absence counts as evidence only when four conditions hold. The first three are
properties of the pass and disable tombstoning outright:

| Condition | Why |
|---|---|
| The pass completed | A scan stopped by `--max-files` saw a prefix of the tree; everything after the cutoff is missing for the wrong reason. |
| The pass was not resumed | A resumed scan deliberately skips everything at or before its cursor. |
| Nothing was skipped | An unreadable directory is a hole in coverage. Files beneath it are unobserved, not gone. |

The fourth is per-locator: a file newly excluded by `.gystignore` disappears from
a scan *exactly* like a deleted one. Tombstoning it would assert it was removed
from the source when it was only removed from view. Those are suppressed and
counted separately, including everything beneath a newly ignored directory.

When tombstoning is refused the scan says so rather than staying silent:

```
absent     not checked: scan was truncated by --max-files; unseen files are unobserved, not absent
```

Absence is recorded at confidence 0.9, not 1.0. A complete pass failing to find
a file is strong evidence of deletion but not proof — it may have been moved, a
mount may have been absent, or permissions may have changed.

### The same bug, one layer down

`TombstoneID` was originally keyed on source and locator alone, which is the
identical mistake day 2 fixed in `DeriveID` — and it bites harder here. Delete a
file, restore it, delete it again, and a state-derived id collides with the
first tombstone: nothing is appended and the projection shows the file present
forever. Tombstone ids now include the seq of the observation whose subject went
missing. Verified end to end:

```
 5  file.content_fingerprint  obs_f510448f57a1
25  artifact.absent           obs_1badbf991a8c
43  file.content_fingerprint  obs_7c67f1307789
44  artifact.absent           obs_3fb0d7fd4220
```

Four states, four distinct observations. Worth generalising: any identifier
derived from observed state has this failure mode wherever a state can recur.

## Commit/file reconciliation

The same bytes were observed twice — by the local-folder connector as a path
with an mtime, by the Git connector as a path inside a commit — and nothing
related them.

They could not be related because **a locator is only meaningful relative to its
source's root**, and roots were never recorded. The scan calls the file
`firmware/src/main.c`; Git calls it `src/main.c`. They are the same file only
because the repository sits at `firmware/` inside the scanned tree. A new
`sources` table records each source's root, and matching resolves both sides to
absolute paths rather than comparing locator strings.

Commits get their own projection (`commits`, `commit_files`) rather than being
forced into `current_files`, and each matched path produces a `contains`
relation citing observations from both connectors. A commit touching a path no
scan has observed is counted as unmatched and no relation is invented.

`gyst explain` now answers which commits changed a file:

```
changed by (git)
  COMMIT        WHEN        AUTHOR         MESSAGE                    EVIDENCE
  95b5fdd15884  2024-08-11  Gyst Fixture   Add watchdog init          obs_d26d01f16ae7
  f80aff2cefa7  2024-08-11  Gyst Fixture   Initial firmware skeleton  obs_aa264ad9028a
```

## Two bugs found while wiring it up

**Relation ids ignored source.** `relationID` hashed the policy version, type,
and the two locators, but not which source each locator belonged to. Two sources
scanning trees that both contain `widget_rev3.pdf` produced the same id for
distinct relations about distinct files, and the second insert failed on the
primary key. Surfaced only because a stray `--max-files` probe left a second
source in the projection.

**`explain` gave up too early.** With no identity policy active it returned
before printing Git history or relations — neither of which depends on an
identity interpretation.

Also: `relations` could not represent anything derived from source-native
evidence, because its primary key required an identity policy version. A commit
touching a file is not an identity decision, so naming a policy would imply a
grouping that was never made. The key moved to `relation_id` alone and the
policy version is now nullable.

## Both earlier criteria still hold

With tombstones and commits in the log:

- Replay from seq 0 reproduces the projection fingerprint exactly.
- All five identity profiles rebuild grouping with the log byte-identical.
- A deleted file drops out of grouping entirely (`artifact_members` count: 0).

## Known gaps

- **Deletion and rename are indistinguishable.** A moved file is a tombstone plus
  an unrelated new file. The content digest to connect them is already recorded,
  so rename detection is available but not implemented.
- **Reconciliation is one-directional and full-sweep.** It re-walks every commit
  on each run rather than resuming, and matches only commit → scanned file, not
  scanned file → commit.
- **Git observes commits, not file versions.** There is no observation of a
  file's content *at* a commit, so Gyst cannot answer what a file looked like at
  a past revision — only which commits touched it.
- **No `generated-from` relations** still, carried from day 3.
- **Tombstones only for the local-folder connector.** A commit deleting a file
  produces no absence claim from the Git side.
- **Source roots are trusted, not verified.** If a source is re-registered with a
  different root, existing locators silently change meaning.
