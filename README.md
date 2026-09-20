# Gyst

**Get your systems together. Git your shit together.**

Gyst is a free and open-source system for finding, understanding, and
coordinating the work products of engineering and manufacturing organizations.
It meets teams in the tools they already use—Git repositories, local folders,
cloud drives, spreadsheets, and kanban systems—then connects those artifacts to
projects, parts, people, costs, schedules, decisions, and production feedback.

Gyst is not intended to replace every source system. It is a headless,
local-first engineering observability and provenance layer with optional user
interfaces for different roles. It is the open-source engine within the broader
**Work Glue** idea: connect the work without demanding that an organization move
all of it into a new system of record.

## Status

This repository contains a pre-alpha working skeleton and its design record. The
current implementation can scan local folders, record immutable observations,
rebuild projections, observe Git history, apply reversible identity profiles,
explain provenance, record tombstones, reconcile Git commits with working files,
and conservatively detect renames. It is not yet ready for production data.

Project documents include:

- [Product definition](docs/product.md)
- [System architecture](docs/architecture.md)
- [Round-two design](docs/design-round-2.md)
- [Delivery roadmap](docs/roadmap.md)
- [Open questions and decisions](docs/decisions.md)
- [Project strategy and sustainability](docs/strategy.md)
- [Plan: first three days](docs/plan-week-1.md)
- [Day 2 notes: walking skeleton](docs/day-2-notes.md)
- [Day 3 notes: identity profiles and Git provenance](docs/day-3-notes.md)
- [Day 4 notes: tombstones and reconciliation](docs/day-4-notes.md)
- [Day 5 notes: rename detection](docs/day-5-notes.md)
- [Day 6 notes: scan passes](docs/day-6-notes.md)
- [Day 7 notes: source location and discovery](docs/day-7-notes.md)
- [Day 8 notes: projects and membership](docs/day-8-notes.md)
- [Day 9 notes: findings](docs/day-9-notes.md)
- [Designer brief](docs/designer-brief.md)
- [Design system and first-view plan](docs/design-system.md)
- [Handoff log between design and code](docs/handoff.md)

Conventions for the two AI agents working in this repository are in
[`AGENTS.md`](AGENTS.md).

The one-page project site lives in [`site/`](site/). It describes the problem,
the current workflow, the self-hosting boundary, and the prototype's honest
status. See [`site/README.md`](site/README.md) to run or build it locally.

## Working principles

1. Meet users where they are; adoption must not require a big-bang migration.
2. Leave authoritative data in its authoritative system unless a user opts in
   to managed storage.
3. Never run Git directly inside a third-party synchronized folder.
4. Every derived fact must retain provenance: source, version, observation time,
   extractor, and confidence.
5. Useful read-only visibility comes before write-back automation.
6. Headless core first; role-specific experiences consume stable APIs.
7. Local-first and self-hostable by default, with explicit security boundaries.
8. Prefer adapters and upstream projects over permanent forks.
9. No account, vendor cloud, license server, or internet connection is required
   for core operation.
10. Observe work products, not worker productivity; never turn artifact activity
    into opaque employee rankings.

## Near-term target

The first useful release is a Windows-first engineering observatory for one
engineering team. It inventories local folders and Git repositories, detects
recent changes, identifies generated and duplicate files, associates artifacts
with projects, and understands enough KiCad and PDF to prepare an inspectable
release candidate. It exposes the result through a CLI and API. It does not yet
promise bidirectional cloud sync, arbitrary CAD support, PLM, ERP, or autonomous
file moves.

## Deployment promise

Gyst treats self-hosting and air-gapped operation as architectural constraints,
not enterprise add-ons. A documented release must be installable, usable,
inspectable, and updatable without a vendor-operated service. Connected services
may be supported, but they may not become dependencies of the core workflow.

## License

Gyst is released under the [MIT License](LICENSE). The license deliberately
optimizes for inspection, adoption, local modification, and use inside
organizations that cannot send engineering data to a managed service.
