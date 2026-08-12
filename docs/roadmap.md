# Delivery roadmap

The roadmap is organized around evidence and usable vertical slices, not feature
counts. Each phase should end with a demo against realistic, messy project data.

## Phase 0 — Discovery and contracts (2–4 weeks)

Goal: validate the wedge and eliminate architecture-changing unknowns.

- Interview 8–12 people across engineering management, engineering, document
  control, and manufacturing at 2–3 organizations.
- Collect sanitized examples of folder trees, Git repos, BOM spreadsheets,
  release packages, and manufacturing feedback.
- Write a threat model and data classification matrix.
- Define the observation envelope, source identity, artifact identity, relation,
  and connector capability schemas.
- Spike reversible identity profiles across rename/copy/suffix conventions, a Git
  history importer, XLSX extraction, and event replay into projections.
- Spike Windows service/per-user operation, filesystem events, installer, upgrade,
  path semantics, locked files, and long paths.
- Spike supported KiCad CLI versions, native metadata extraction, deterministic
  outputs, and semantic comparison.
- Choose license, governance, language, and repository layout.

Exit criteria:

- One documented pilot team and workflow.
- Versioned schemas with fixtures from at least three source types.
- Measured scan/hash performance on a representative dataset.
- Architecture decisions recorded for the choices above.

## Phase 1 — Windows project observatory (6–8 weeks)

Goal: answer “where is it?” and “what changed?” for a single team without moving
their data.

- Headless Windows edge agent/service with configured-root scanning, `.gystignore`,
  layered policy, cursors, and resource budgets.
- Native Git and local-folder connectors.
- PostgreSQL event store and rebuildable projections.
- File metadata, content fingerprint, rename/copy candidates, Git provenance,
  and generated-file rules.
- Explicit many-to-many project manifests plus suggested membership.
- Reversible file identity profiles with preview and explainability.
- CLI and API for artifact search, recent changes, provenance, and findings.
- Minimal web experience for project inventory and recent activity.

Exit criteria:

- Index 100k files incrementally without data loss or unbounded rescans.
- Restart/replay produces equivalent projections.
- Every displayed fact links to its source observation.
- The pilot team can find canonical artifacts faster than its current process.
- No source mutation is possible in the default configuration.

## Phase 2 — KiCad, records, BOMs, and releases (8–12 weeks)

Goal: create a trustworthy engineering-to-manufacturing handoff.

- CSV/XLSX extraction with workbook/sheet/cell provenance and validation.
- Typed authoritative Records with schema, stable IDs, validation, history, and
  grid/form views; evaluate a Grist adapter during implementation planning.
- Configurable BOM mapping, normalization, hierarchy, and stable part identity.
- KiCad extraction, generation runs, validation, and configured release outputs.
- Named release snapshots containing exact source versions and derived outputs.
- Semantic BOM and document-set diffs.
- Manufacturing read view, feedback threads, ownership, and acknowledgement.
- Immutable audit log and signed/exportable release manifest.

Exit criteria:

- Reproduce a release from its manifest or clearly report unavailable inputs.
- Explain each BOM diff back to workbook cells and source versions.
- Reproduce KiCad-generated outputs using the pinned release recipe and supported
  tool version, or state precisely why reproduction is unavailable.
- Manufacturing feedback reaches an accountable project owner and closes with an
  auditable disposition.

## Phase 3 — Cloud and work-system connectors (8–12 weeks)

Goal: span the real toolchain without using synchronized folders as an API.

- Implement one cloud drive selected by pilot demand.
- Implement one work tracker/kanban provider selected by pilot demand.
- Webhook/event ingestion plus periodic reconciliation and rate-limit handling.
- Source-native authorization, tombstones, retention, and stale-connector status.
- Cross-source project/activity timeline.

Exit criteria:

- Connector survives dropped/duplicate/out-of-order events and rebuilds state.
- No Git repository is created within provider-synchronized storage.
- Permission revocation removes access from derived queries predictably.

## Phase 4 — Planning and portfolio views

Goal: support managers and directors with explainable rollups.

- Milestones, schedules, capacity/labor inputs, and cost model primitives.
- Configurable calculations for COGS and schedule/capacity scenarios.
- Dependency and risk propagation with source freshness/confidence.
- Manager workload/hygiene view and portfolio status view.
- Exportable reports and stable analytics API.

Exit criteria:

- Every rollup exposes formula, inputs, freshness, and authority.
- Scenario edits cannot be confused with authoritative operational data.
- Pilot leadership uses the view in a recurring review without parallel manual
  aggregation for the covered scope.

## Phase 5 — Ecosystem and federation

Goal: make Gyst an extensible FOSS platform rather than a closed application.

- Stable connector/extractor SDKs and compatibility test suites.
- Sandboxed extension packaging and permission manifests.
- MCP server over stable application services.
- Selective federation of signed releases, feedback, and provenance.
- Signed air-gap transfer bundles and import/export policy tooling.
- Optional managed-folder workflows after recovery and conflict behavior are
  thoroughly specified and tested.

## First backlog

1. Write three end-to-end user journeys from actual interviews.
2. Draft JSON Schemas for `Observation`, `ArtifactRef`, `Relation`, and `Finding`.
3. Create a synthetic messy engineering dataset and expected inventory fixture.
4. Spike stable identity for local files through edit, rename, copy, and replace.
5. Spike incremental Git import without assuming the Git provider.
6. Spike XLSX formulas, merged cells, hidden sheets, and table extraction.
7. Prove event replay and idempotency in the smallest end-to-end service.
8. Benchmark full and incremental scans with content-policy variations.
9. Produce the initial threat model.
10. Specify the reconciliation model and create binary choose/keep-both fixtures.
11. Specify connected and air-gapped enrollment, update, and transfer flows.
12. Demo `gyst scan`, `gyst changes --since 1h`, `gyst explain <artifact>`, and a
    KiCad release-candidate comparison.

## Explicit non-goals for the first release

- Automatic repository splitting or monorepo conversion.
- Bidirectional sync.
- Automatic project boundary changes.
- Executive dashboards built from guessed cost/schedule data.
- AI-generated mutations.
- Supporting CAD other than KiCad natively; unsupported designs use file metadata
  and PDF-based review.
