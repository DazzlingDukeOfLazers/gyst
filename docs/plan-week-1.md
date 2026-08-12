# Plan: first three days

This plan covers one slice of Phase 0, not the phase. It selects the work that
retires the most architecture-changing unknowns per day and forces the prose
contracts in `architecture.md` and `design-round-2.md` to become executable.

## Standing constraints

Two conditions shape the selection below.

**Development host is macOS; the agent target is Windows.** The roadmap's
riskiest Phase 0 spikes — service and per-user operation, installer, upgrade,
locked files, long paths — cannot be attempted without a Windows VM. All three
days are therefore OS-agnostic. The Windows spike is a separate block with its
own environment prerequisite and does not belong inside this window.

**Interviews have a multi-week lead time and no code dependency.** Backlog item
1 is the only Phase 0 item where a three-day delay costs three weeks. Outreach
goes out on day 1 morning so replies accumulate while the skeleton is built.

## Day 1 — Make the contracts real

`Observation`, `ArtifactRef`, `Relation`, and `Finding` exist only as prose.
Until they are schemas with fixtures, every downstream decision is guesswork.

| Block | Work |
|---|---|
| Morning | Repository layout and Go module skeleton (ADR-001). Commit the Go decision or discard it; `design-round-2.md` still lists it as a candidate, and days 2–3 assume it. |
| Morning | Send interview outreach. |
| Afternoon | Versioned JSON Schemas for the four envelope types, carrying the fields the architecture invariants require: source locator plus native version, extractor name and version, input digest, confidence, observation time, and identity-policy version. |
| Afternoon | Synthetic messy dataset (backlog item 3) and its expected-inventory fixture. |

The dataset must contain a Git repository, unmanaged folders, a
`widget_rev2.pdf` / `widget_rev3.pdf` pair, a `connector_123.pdf` /
`connector_124.pdf` pair, a duplicated BOM workbook, and a directory of
generated Gerber output. The two suffix pairs are the cases that must break the
identity profiles on day 3; they are fixtures, not decoration.

Exit criteria:

- One valid instance of each schema, hand-written against a real file in the
  fixture tree.
- Language and repository layout recorded as a decision rather than a candidate.

Deliberately excluded: KiCad, PDF, and XLSX extraction; policy layering.

## Day 2 — Walking skeleton

This day retires backlog items 7 and 8 together and produces the evidence for
ADR-004.

| Block | Work |
|---|---|
| Morning | PostgreSQL in a container. Append-only observations table plus a compact current-state projection. |
| Morning | Local-folder connector implementing only `discover(cursor) -> observations, next_cursor`, with a persisted cursor, `.gystignore` handling, and a file-stability check before hashing. |
| Afternoon | Idempotent projector; `gyst scan` and `gyst changes --since 1h`. |
| Afternoon | Full and incremental scan benchmarks over a scaled fixture tree, with metadata-only and fingerprint content policies compared. |

Use PostgreSQL rather than SQLite here. The solo profile permits SQLite, but it
also permits avoiding the ordering and replay questions this day exists to
answer.

Exit criteria:

- Dropping the projection, replaying the log, and rebuilding produces an
  equivalent projection.
- A second scan across an unchanged tree emits no new observations.

Sizing risk: this is the fullest of the three days. If the event-store schema
consumes the morning, drop the benchmark and carry it into day 3 rather than
compressing the projector.

## Day 3 — Identity profiles and Git provenance

| Block | Work |
|---|---|
| Morning | Implement the `Content/path exact`, `Suffix-as-version`, `Suffix-as-identity`, and `Compare-set` profiles. Grouping is a projection rebuilt from the log and never a rewrite of it. |
| Morning | The preview and explanation path: show the grouping and the reason for each match before activation, and confirm that low confidence falls back to `Compare-set` instead of inferring supersession from a numeric suffix. |
| Afternoon | Git connector: commits as observations, with `generated-from` and `supersedes` relations. |
| Afternoon | `gyst explain <artifact>`, where every displayed fact links to its supporting observation. |

Exit criteria:

- Switching a root from `Suffix-as-version` to `Suffix-as-identity` rebuilds the
  grouping and leaves the observation log byte-identical. This proves the
  reversibility claim in `design-round-2.md` rather than asserting it.

## Out of scope for this window

KiCad, spreadsheet and BOM extraction, releases, Records, policy layering,
authorization, and every Windows-specific spike. The threat model and license
decision are also excluded: both are decision work rather than build work, and
including them would displace the skeleton.

## Known weakness in the plan

Without an identified pilot team, the Phase 0 exit criterion calling for
measured scan performance on a representative dataset is measured against an
invented one. The schema and replay work stands regardless, but the day 2
benchmark numbers remain provisional until real project data is available.
