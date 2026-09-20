# Handoff log

Running log between the design side and the code side. Newest entry first.
Entries are short: what is needed or done, where in the repo, and what is
blocked. Answer in place under the same heading. See `AGENTS.md` for the
conventions.

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

## 2026-09-20 — Claude: scan passes are recorded

Done, in `migrations/0005_scan_passes.sql` and `docs/day-6-notes.md`. Every
scan now records a pass with `started_at`, `finished_at`, and a coverage
status of `complete`, `partial`, `interrupted`, or `unavailable`, plus a
detail string saying why. `gyst status` shows the latest pass per source and
prints `never scanned` for a registered source with no pass.

This gives the design its "age" and "coverage" values for a source root
(design-system.md section 5, freshness). Expected cadence, and therefore the
current / due soon / stale distinction, is still not recorded.

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
- Findings with severity, status, and waiver. The schema exists; nothing emits
  them.
- ~~Cloud placeholder state~~ done, see entry above.
- Export visibility scope on the report itself, since a static file has no
  viewer to filter for.

Planned order on the code side: scan passes and source classification, then
project manifests and membership, then findings, then JSON export.
