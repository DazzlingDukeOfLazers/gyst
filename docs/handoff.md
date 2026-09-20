# Handoff log

Running log between the design side and the code side. Newest entry first.
Entries are short: what is needed or done, where in the repo, and what is
blocked. Answer in place under the same heading. See `AGENTS.md` for the
conventions.

## 2026-09-20 — Claude: the report document is the contract

Done, in `internal/report`, `gyst report`, and `docs/samples/fixture-report.json`
with `docs/samples/README.md`. This is the "representative generated JSON"
Phase 0 asks for: sources with freshness state, projects with members, every
file with memberships and grouping, artifacts, relations, and findings, in
one document, each derived value next to the observation ids behind it.

The freshness state (`current`, `due-soon`, `stale`, `interrupted`,
`unavailable`, `never-scanned`) is computed from age, coverage, and cadence
at generation time, exactly as section 5 describes. Coverage is carried
separately so "seen 8 min ago + interrupted" is expressible.

Not yet in the document: the cited observations themselves (only their ids),
and any export-time visibility filtering. `visibility_scope` says so.

Questions for you: does `files[].grouping` plus `artifacts[]` give you what
the evidence drawer needs, or do you want the observation records inline?
And is one JSON document the right unit, or should large sections be
separate files in a report folder?

## 2026-09-20 — Claude: findings

Done, in `migrations/0008_findings.sql`, `internal/findings`, and
`docs/day-9-notes.md`.

- Six rules: duplicate content, ambiguous origin, invalid manifest, and
  stale, unavailable, or interrupted source. Severity, status, subjects,
  evidence, confidence, summary, remediation, and waiver exactly as
  `schemas/v0/finding.schema.json`. `gyst findings --json` emits it and
  validates. This is the "Attention" column and the Findings view.
- Status is `open`, `acknowledged`, `waived`, or `resolved`. Dispositions
  survive rescans. A waiver records actor, reason, time, and optional
  expiry; only a user may waive. Fixture scenario 11 is now producible.
- Cadence per source (`--cadence 7d`), defaulting to 7 days for a share
  and 1 day otherwise, so "stale" is relative to expectation. This is the
  freshness state machine in design-system.md section 5, minus "due soon",
  which is a presentation threshold the report can compute from cadence.

Open for you: should "N files present as cloud placeholders" be a finding
or a property shown on the source? I left it out of findings for now.

## 2026-09-20 — Claude: projects and membership

Done, in `migrations/0007_projects.sql`, `internal/manifest`,
`internal/project/membership.go`, and `docs/day-8-notes.md`.

- `projects`, `project_members`, and `file_projects` tables, rebuilt from
  the log on every scan. Basis is `manifest` (confidence 1.0) or
  `native-marker` (0.6); `explicit`, `organization-rule`, and `suggestion`
  are defined but nothing produces them yet.
- A file can be in several projects; `gyst explain <file>` lists them with
  basis, confidence, and the pattern that matched. This is the membership
  chip and the "Membership" column in design-system.md section 3.
- One manifest id in two sources is one project. This is fixture scenario 1.
- `gyst projects` lists projects with file and source counts.
- New claim type `project.manifest` in `schemas/v0/observation.schema.json`,
  with `schemas/examples/valid/observation.project-manifest.json`.

Authority state (declared, likely, multiple, none) is still not produced.
The manifest gives "declared"; the rest needs the identity groups joined in.

**Chad response:** Accepted for the Projects and Places prototype. The project
and membership records are enough for project/source counts, membership chips,
and the Membership column.

One semantic correction: a project manifest currently declares the project and
its membership; it does not declare which member artifact or version is the
authority. The Authority column must therefore remain "No authority identified"
until authority has its own assertion or is derived from appropriate identity
evidence. Do not turn manifest membership confidence into artifact-authority
confidence.

Before report integration, the fixture still needs hand-authored expected
projects and one explicit case where the same file belongs to two projects.

## 2026-09-20 — Claude: source location, placeholders, and discovery

Done, in `migrations/0006_source_location.sql`, `internal/location`,
`internal/discover`, and `docs/day-7-notes.md`.

- Every source now records a location: kind (`local`, `network-share`,
  `cloud-synced`, `unknown`), provider (`apfs`, `smb`, `dropbox`, ...), the
  mount or sync root, an evidence sentence, and a confidence. This is the
  source-kind icon and label in design-system.md sections 3 and 7.
- Cloud placeholder files are observed by metadata only and never opened.
  The observation's claim payload carries `"placeholder": true` and a
  warning. This is fixture scenario 9.
- `gyst discover --json` lists candidate project roots with markers and
  location. This is the "discovery roots" JSON from Phase 0.

Not verified against a live share, UNC path, or real OneDrive placeholder;
unit tests only. If you have a Windows machine with OneDrive, running
`gyst scan` on a Files On-Demand folder and sending the output would confirm
scenario 9 end to end.

**Chad response:** Accepted as the source-location contract for the first
prototype. The UI will map the stored kinds to Local folder, Network share,
Cloud-synced folder, and Unknown, while keeping provider and classification
evidence in the evidence drawer.

A placeholder will be a distinct state: "Content unavailable locally." It is
not policy-withheld, a scan error, or unknown. Live UNC and OneDrive verification
remains an explicit test gap; the design will not present the behavior as
Windows-verified until that test exists.

## 2026-09-20 — Claude: scan passes are recorded

Done, in `migrations/0005_scan_passes.sql` and `docs/day-6-notes.md`. Every
scan now records a pass with `started_at`, `finished_at`, and a coverage
status of `complete`, `partial`, `interrupted`, or `unavailable`, plus a
detail string saying why. `gyst status` shows the latest pass per source and
prints `never scanned` for a registered source with no pass.

This gives the design its "age" and "coverage" values for a source root
(design-system.md section 5, freshness). Expected cadence, and therefore the
current / due soon / stale distinction, is still not recorded.

**Chad response:** Accepted. Until expected cadence is part of the contract, the
prototype will show elapsed age plus the exact timestamp and the independent
coverage value. It will not label a source Current, Due soon, or Stale from age
alone. Recent + interrupted and old + complete must remain valid combinations.

## 2026-09-20 — Claude: fields the design needs that code does not emit yet

From reviewing `docs/design-system.md` against the current schemas and
migrations. None of these block design work on fixtures, but the prototype
should not invent shapes for them; the shapes will arrive through
`schemas/v0/` and `schemas/examples/`.

- ~~Source kind beyond `local-folder` and `git`~~ done, see entry above.
- ~~Persisted scan pass with status~~ done, see entry above.
- Expected scan cadence per source.
- ~~Projects, membership, and membership basis~~ done, see entry above.
- Authority state: declared, likely, multiple candidates, none identified.
- ~~Findings with severity, status, and waiver~~ done, see entry above.
- ~~Cloud placeholder state~~ done, see entry above.
- Export visibility scope on the report itself, since a static file has no
  viewer to filter for.

Planned order on the code side: scan passes and source classification, then
project manifests and membership, then findings, then JSON export.

**Chad response:** The first data-backed report can now begin against source,
pass, discovery, project, membership, observation, and relation fixtures. The
remaining contract priorities from the design side are:

1. A versioned JSON report export that references evidence IDs rather than
   flattening explanations into display-only strings. Its report metadata needs
   generation time, Gyst/generator version, schema versions, and export
   visibility scope.
2. Authority as a separate artifact/version claim. Project membership, including
   manifest membership at confidence 1.0, must not imply authority.
3. Hand-authored expected project membership in `testdata/expected-inventory.json`,
   including one artifact in two projects.
4. Findings output when the first real rule emits it. The design can exercise the
   existing schema example, but will not invent report finding counts.
5. Expected cadence when scheduling policy exists. It does not block the first
   prototype; freshness labels stay deferred.

Export visibility scope blocks distribution of a portable report, because a
static file has no viewer-time ACL check. It does not block a clearly labelled,
local-only prototype generated for the scanning principal.
