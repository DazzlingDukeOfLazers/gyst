# Day 3 notes: identity profiles and Git provenance

Five reversible identity profiles, a grouping projection with derived relations,
native Git provenance, and `gyst explain`.

## Exit criterion

Switching profiles must rebuild grouping and leave evidence untouched.
`gyst identity verify` activates all five in turn against the fixture and
fingerprints the observation log after each:

| Profile | Artifacts | supersedes | compare-set-with | Log after |
|---|---|---|---|---|
| content-path-exact | 19 | 0 | 0 | unchanged |
| suffix-as-version | 17 | 1 | 1 | unchanged |
| suffix-as-identity | 19 | 0 | 0 | unchanged |
| canonical-name | 15 | 0 | 4 | unchanged |
| compare-set | 15 | 0 | 4 | unchanged |

The log stayed byte-identical across all five. Grouping is a projection keyed by
policy version, so activating a profile writes a new version beside the old one
rather than mutating it — a release that pinned an earlier interpretation can
still resolve it.

## The trap, handled

`connector_123.pdf` and `connector_124.pdf` are distinct part numbers.
`widget_rev2.pdf` and `widget_rev3.pdf` are a revision series. They are
structurally identical: a name, a separator, a number.

What separates them is the marker word. An explicit `rev`/`v` scores 0.88 and
clears the 0.80 supersedes threshold; a bare trailing number scores 0.35 and
does not. Both pairs still group under `suffix-as-version` — the profile is
doing what it says — but only one produces an ordering:

```
supersedes        0.88  widget_rev3.pdf   -> widget_rev2.pdf
compare-set-with  0.35  connector_124.pdf -> connector_123.pdf
```

The threshold is enforced three times over: in `RelationFor`, in the JSON Schema,
and as a `CHECK` constraint on the relations table. It is a product commitment,
not an implementation detail of one code path.

Still unresolved: **0.80 is a number I chose**, not one derived from the design
documents. It is the single value deciding whether `connector_124` appears to
supersede `connector_123`. It belongs in `decisions.md`.

## Three bugs the fixture caught

**A revision marker embedded in a word.** The first pattern allowed an optional
separator before the marker, so the trailing `r` of `connector_123` served as
one and the file read as "version 123 of *connecto*". A marker has to be its own
token; the separator is now required.

**Grouping ignored file type.** `widget.kicad_pcb` collapsed into the
`widget_rev2/rev3` PDF series, because all three share the base name `widget`.
That produced a nonsense relation asserting both "carry version-like tokens"
about a file with no version token. Grouping keys now carry the extension. The
tradeoff: a supplier who ships `drawing_rev2.pdf` then `drawing_rev3.dwg` gets
two artifacts. Refusing to merge across formats is the safer default.

**A fixture expectation that contradicted its own note.** The day 1 fixture said
`Assembly Notes v2 FINAL.pdf` should group under `Assembly Notes` in
`suffix-as-version`, while its note said "FINAL is not a revision scheme". Both
cannot hold: a strict profile must not strip a qualifier word to force a
grouping. Only `compare-set`, which never orders anything, may group across it.
The expectation was wrong and was corrected.

That is twice now the hand-authored expectations have been wrong and the
implementation right. This is the fixture working, not failing — an expectation
that can never contradict the code cannot catch anything — but it does mean the
day 1 expectations were written with less care than the code they check.

## Git provenance

`gyst git --repo <path>` reads commits through `git log` and emits one
observation per commit. Commit ids are content digests, so re-observation is
naturally idempotent and `DeriveID`'s prior-seq argument is zero: a commit
cannot revert to an earlier version of itself.

The connector never writes — no checkout, no fetch, no config change — and runs
with `GIT_CONFIG_GLOBAL=/dev/null` so a user's local settings cannot perturb what
is observed.

One bug found immediately: the projector folded commits into `current_files`,
producing rows keyed `src@<oid>` with null sizes that the identity profiles then
tried to group as filenames. `current_files` is a file projection; a commit is an
artifact but not a file. The projector now skips non-file kinds while still
advancing its cursor past them.

## gyst explain

Every line cites the observation behind it. For the trap file it reports the
grouping, the rule and confidence that produced it, the sibling it grouped with,
and the relation — with the observation ids supporting each. A fact with no
citation is reported as unknown rather than filled in.

## Known gaps

- **No `generated-from` relations yet.** The fixture declares four generated
  outputs and the schema models the relation, but nothing derives it. It needs
  the `generation.run` claim, which is Phase 2 work.
- **Commits have no projection of their own.** They are in the log and queryable,
  but there is no `commits` table and no relation tying a commit to the files it
  touched, so `explain` cannot yet say which commit last changed a file.
- **The two views of a Git working file do not reconcile.** `firmware/README.md`
  is observed by the local-folder connector as bytes at a path and by the Git
  connector as part of a commit. Both observations exist; nothing relates them.
- **Profile scope is unused.** The `rule` struct carries a `scope` field and the
  round-two design calls for scoped rules, but every rule currently applies
  globally. Per-directory profiles are the obvious next step.
- **Tombstones still missing**, carried over from day 2. Deleting a file leaves
  its projection row `present`, so a deleted file still appears in groupings.
- **Ordering assumes integers.** `rev2` before `rev3` works; `revA` before `revB`,
  or `1.2.10` after `1.2.9`, does not.
