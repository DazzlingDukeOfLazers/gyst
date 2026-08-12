# Decisions and open questions

This register captures product choices and the implementation questions that
remain. A decision moves to an architecture decision record (ADR) when code or a
public contract depends on it.

## Accepted direction

| Topic | Decision | Consequence |
|---|---|---|
| Product category | Digital-thread integration and provenance layer | Integrate with incumbent systems instead of replacing all of them |
| Initial wedge | Find current files, improve hygiene, compare BOMs, and create releases | One end-to-end engineering/manufacturing workflow precedes broad dashboards |
| Product/project | Product is a sellable line item; project is any useful artifact collection | Project membership is many-to-many and independent of folders/repos |
| Source behavior | Observe by default; assist/manage are explicit per source | No surprise moves, commits, or writes |
| Git strategy | Native integration and optional private mirrors; no forced monorepo | Gitea/Forgejo are providers, not initial fork bases |
| Synced drives | Provider API; never embed Git in synced directories | Reconciliation uses native item IDs/events |
| Installation | Users install local tools; IT/admins deploy servers; org owners administer them | Windows agent and understandable forge-like administration are first-class |
| Platform | Windows is first-class and the initial agent target | Service/per-user modes, installer, locked-file and path testing enter Phase 0 |
| CAD scope | KiCad native support first; PDF comparison fallback | Other CAD is indexed but not semantically loaded/diffed initially |
| Generated files | First-class artifacts tied to generation runs | Releases pin inputs, tool/config versions, logs, and output hashes |
| Physical revision | Integer revisions are the initial convention | Revision schemes remain organization-configurable |
| Parts | Internal item/PN plus manufacturer numbers, distributor offers, and alternates | Supplier identifiers do not replace internal identity |
| Feedback | Comments, nonconformances, deviations, and change proposals are distinct workflows | Shared primitives, separate state machines and dispositions |
| Structured data | Typed authoritative Records replace parallel free-form working copies | Spreadsheet import/export remains supported with provenance |
| Cost/labor/schedule | Managers/directors define schemas over safe primitives | Rollups expose schema/formula versions, inputs, and freshness |
| Deployment | Connected and fully air-gapped models | Same core; air gaps use signed asynchronous transfer bundles |
| Server shape | Separate headless modular service, not a Gitea fork | Stable API serves CLI, web, integrations, and later MCP |
| Data flow | Immutable observations plus rebuildable projections | Reclassification never rewrites evidence or releases |
| Metadata/search | PostgreSQL first; add specialized projections only at measured gates | Avoid premature search/graph infrastructure |
| Binary storage | Source-native by default; optional S3-compatible CAS | Gyst need not possess content to index provenance |
| Authorization | Connector reach and user visibility are separate | Evidence-aware ACL filtering prevents derived metadata leaks |
| MCP | Adapter over ordinary application services | Initial tools are read-only and never bypass policy |

## Proposed direction awaiting prototype confirmation

| Topic | Proposal | Confirmation gate |
|---|---|---|
| Agent/server/CLI language | Go | Windows service, scanner, packaging, and concurrency spike |
| Web client | TypeScript | Walking-skeleton UI and API client generation |
| License | AGPL-3.0-or-later for server; compatible SDK license | Contributor/dependency review and governance decision |
| Structured grid reuse | Evaluate self-hosted Grist behind an adapter | Access, offline, licensing, deployment, audit, and API fit spike |
| File identity | Reversible scoped identity profiles | Test with real supplier/customer naming datasets |

## Policy files to validate

The current naming proposal is `.gyst/project.yaml`, `.gyst/policy.yaml`,
`.gyst/release.yaml`, and `.gystignore`. User testing must confirm that these are
discoverable and understandable. Central policy is required for sources that
cannot contain configuration. The most restrictive effective rule wins.

## Unresolved product questions

1. Which of finding, hygiene, BOM diff, and release creation is the acquisition
   hook, and which is the retention hook? Test rather than assuming they are equal.
2. Who can approve product/item revisions and releases in the pilot organization?
3. Which KiCad major versions and exact release outputs must the first pilot use?
4. What minimum workflow states and approvals distinguish a nonconformance,
   deviation, and change proposal in the pilot?
5. Should authoritative Records be native to Gyst, an integrated FOSS grid, or a
   thin contract with more than one compatible implementation?
6. What must remain on-device versus within a facility? Organization policy owns
   the answer, but defaults and onboarding language still require validation.
7. Which compliance/export-control evidence should Gyst produce without claiming
   that using Gyst itself makes an installation compliant or certified?

## Unresolved technical questions

- File identity profile grammar, precedence, migration, and ambiguity UX.
- Stable local file identity across Windows volumes, network shares, rename, copy,
  atomic replace, and unavailable files.
- Exact observation envelope, ordering, compaction, replay, and retention rules.
- Supported KiCad semantic model and deterministic-generation boundary.
- PDF render/diff normalization across producer versions and fonts.
- Record formula language, migrations, test fixtures, and sandbox.
- Source ACL snapshot/mapping behavior when the upstream API is incomplete.
- Reconciliation behavior when a source changes during human review.
- Air-gap signing/trust bootstrap, revocation, redaction, and schema negotiation.
- Organization/instance identity and key rollover for federation.
- PostgreSQL partitioning and authorization-aware search load limits.

## Scale decisions

PostgreSQL is sufficient until profiling at representative data volume and
concurrency proves otherwise. A dedicated search index is justified when tuned
interactive search misses a 500 ms p95 target, search harms ingestion/transaction
work, or independent scaling is required. A graph engine is justified only after
required multi-hop queries have been profiled and miss their product latency goal.

These are trigger conditions, not promises that a particular artifact count will
behave identically on every deployment.

## Compliance stance

Gyst is deployable by organizations in environments with different compliance
needs, including air-gapped or appropriately hosted services. Installation does
not itself confer ISO 9001, AS9100, medical, export-control, or other compliance.
Gyst provides provenance, controlled records, audit exports, and evidence that a
qualified certification/compliance team may evaluate. Documentation and UI must
avoid warranties or certification claims.

## ADRs required before implementation

1. Repository and module layout.
2. License, contributor policy, and dependency acceptance.
3. Observation envelope, artifact/location/version identity, and profile rules.
4. Event storage, ordering, replay, compaction, and retention.
5. Windows agent trust, enrollment, update, service, and per-user model.
6. Effective content/egress policy and hashing privacy.
7. Authorization, source principal mapping, and derived-data visibility.
8. API compatibility and versioning.
9. Connector/extractor packaging and permissions.
10. KiCad support/version/reproducibility contract.
11. Record schema/formula/migration contract.
12. Release manifest, signing, and transfer-bundle format.
