# Gyst

**Get your shit together. Git your shit together.**

Gyst is a proposed free and open-source system for finding, understanding, and
coordinating the work products of engineering and manufacturing organizations.
It meets teams in the tools they already use—Git repositories, local folders,
cloud drives, spreadsheets, and kanban systems—then connects those artifacts to
projects, parts, people, costs, schedules, decisions, and production feedback.

Gyst is not intended to replace every source system. It is a headless,
local-first integration and provenance layer with optional user interfaces for
different roles.

## Status

This repository is at the planning stage. The initial documents are:

- [Product definition](docs/product.md)
- [System architecture](docs/architecture.md)
- [Round-two design](docs/design-round-2.md)
- [Delivery roadmap](docs/roadmap.md)
- [Open questions and decisions](docs/decisions.md)
- [Plan: first three days](docs/plan-week-1.md)
- [Day 2 notes: walking skeleton](docs/day-2-notes.md)
- [Day 3 notes: identity profiles and Git provenance](docs/day-3-notes.md)

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

## Near-term target

The first useful release is a Windows-first project observatory for one
engineering team. It inventories local folders and Git repositories, detects
recent changes, identifies generated and duplicate files, associates artifacts
with projects, and understands enough KiCad and PDF to prepare an inspectable
release candidate. It exposes the result through a CLI and API. It does not yet
promise bidirectional cloud sync, arbitrary CAD support, PLM, ERP, or autonomous
file moves.

## License

License selection is an explicit pre-alpha decision. The current preference is
an OSI-approved copyleft license, with AGPL-3.0-or-later as the leading candidate
for the server and compatible licensing for SDKs and integrations.
