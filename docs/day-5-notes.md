# Day 5 notes: rename detection

A rename leaves no trace of itself. The filesystem reports one file gone and
another present; only the content connects them. Everything here is therefore
inference, and the interesting engineering is in deciding when to refuse.

## When content does not identify anything

Three cases where matching on a digest produces confident nonsense, all of them
already present in the fixture:

**Empty files.** Every zero-byte file in existence shares one digest. A
directory of them yields every possible pairing and none of them mean anything.
`empty.txt` exists in the fixture precisely to catch this. Zero-length files are
excluded from matching outright and counted separately.

**Duplicated content.** `widget_bom.xlsx` and `widget_bom (copy).xlsx` are
byte-identical. Delete one and add two copies elsewhere and there is no basis
for deciding which moved where.

**Many-to-many.** Several files gone and several arrived sharing one digest is a
reorganisation, not a set of renames. Pairing them would be invention.

Only a one-to-one match on a digest unique among both sides is called a rename,
at confidence 0.95. Everything else produces low-confidence `compare-set-with`
candidates instead — the same fallback the identity profiles use for an
ambiguous grouping. Dropping ambiguous cases silently would lose the fact that
something moved at all, which is worse than saying "one of these four".

The candidate list is capped at four per disappearance, and anything beyond that
is reported rather than trimmed quietly: a silently dropped candidate reads as
"considered and rejected".

Observed against the fixture:

```
renamed-from      0.95  archive/conn-123-old.pdf  <- connector_123.pdf
compare-set-with  0.30  archive/bom-a.xlsx        <- widget_bom (copy).xlsx
compare-set-with  0.30  archive/bom-b.xlsx        <- widget_bom (copy).xlsx
empty.txt                                          skipped, no content to match on
```

## Other refusals

- **Renames do not cross sources.** Locators are rooted per source, so identical
  content in two sources is a coincidence of bytes, not a move.
- **Matching is scoped to one scan pass.** A file deleted in March and an
  identical one created in July is not a move in any useful sense.
- **An arrival is a locator's first observation.** A file that merely changed is
  not an arrival, so an edit cannot be mistaken for the destination of a move.

## The bug that made it silently find nothing

Pass scoping depends on every observation in a scan sharing `observed_at`.
`Discover` and `Tombstones` each called `time.Now()` independently, so a
tombstone and the arrival it should pair with landed microseconds apart and were
treated as two different passes. Detection ran, matched nothing, and reported
nothing — the failure mode of a correct-looking implementation that never fires.

The scan now takes one clock reading and every observation it produces, tombstone
or not, carries it. This is worth naming: `observed_at` is doing real work as a
pass identifier, which is more weight than a timestamp should carry. An explicit
pass id would be sturdier.

## A guard worth keeping strict

The schema example for `renamed-from` cites a destination path that the fixture
tree does not contain — correctly, because a move's destination exists only
after the move, and the fixture holds the pre-move tree. The example
cross-checker flagged it.

Rather than weaken the check, cases can now declare `allow_absent_locators` with
a reason. The origin is still verified against the fixture; only the destination
is exempt, and the exemption is visible in the manifest.

## Known gaps

- **Detection is a full sweep.** Every scan re-examines every tombstone in the
  log rather than only the current pass. Correct but quadratic in history.
- **Rename plus edit is undetectable.** Moving a file and changing it in the same
  pass breaks the digest match, and nothing weaker (name similarity, size
  proximity) is attempted.
- **Copies are not distinguished from moves at all.** A file copied rather than
  moved leaves the original present, so no tombstone exists and nothing is
  asserted — correct, but `duplicate-of` is still never emitted.
- **Ambiguous candidates are never resolved.** There is no way for a user to say
  "it was this one", which is exactly the reconciliation flow the round-two
  design describes.
- **A rename does not carry identity across.** The new locator is a new artifact
  under every profile; the `renamed-from` relation records the move but grouping
  does not follow it.
