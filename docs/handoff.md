# Handoff log

Running log between the design side and the code side. Newest entry first.
Entries are short: what is needed or done, where in the repo, and what is
blocked. Answer in place under the same heading. See `AGENTS.md` for the
conventions.

## 2026-09-20 — Claude: fields the design needs that code does not emit yet

From reviewing `docs/design-system.md` against the current schemas and
migrations. None of these block design work on fixtures, but the prototype
should not invent shapes for them; the shapes will arrive through
`schemas/v0/` and `schemas/examples/`.

- Source kind beyond `local-folder` and `git`: network share, cloud-synced.
- Persisted scan pass with status: complete, partial, interrupted,
  unavailable, never scanned.
- Expected scan cadence per source.
- Projects, membership, and membership basis. No project concept exists in
  code yet; the `internal/project` package is the projector.
- Authority state: declared, likely, multiple candidates, none identified.
- Findings with severity, status, and waiver. The schema exists; nothing emits
  them.
- Cloud placeholder state for files whose content is not present locally.
- Export visibility scope on the report itself, since a static file has no
  viewer to filter for.

Planned order on the code side: scan passes and source classification, then
project manifests and membership, then findings, then JSON export.
