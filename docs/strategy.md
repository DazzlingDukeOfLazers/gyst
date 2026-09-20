# Project strategy and sustainability

## The twenty-year idea

Gyst is the open-source engine inside a broader idea originally called **Work
Glue**: engineering work already exists across repositories, folders,
spreadsheets, cloud drives, CAD tools, work trackers, and the knowledge of the
people operating them. The first job is to connect and explain that work, not to
force it into another vendor-owned system.

The command-line name remains Gyst. In professional contexts it may be described
as the **Gyst Engineering Observatory**. "Get your systems together" expresses
the product purpose; "Git your shit together" remains the memorable secondary
reading without implying that every source belongs in Git.

## Strategic position

Gyst is **engineering observability and provenance infrastructure**. It is not a
hosted PLM competitor, a general file-sync service, an employee-monitoring tool,
or a promise to become the authority for every business record.

The initial user is a 5-30 person hardware/software team whose work is split
between Git, local or network folders, KiCad, PDFs, and CSV/XLSX BOMs. The initial
promise is deliberately narrow:

> Find the authoritative engineering files. Explain what changed. Prove what
> went into the release. No migration required.

The acquisition hook is finding the right artifact and identifying release
ambiguity. The retention hook is noticing when that truth changes and preserving
an inspectable history.

## Constitutional constraints

The following are product guarantees rather than pricing-plan features:

1. Core operation requires no hosted account, license server, telemetry service,
   cloud identity, remote model, or internet connection.
2. A clean offline installation can inventory a project, explain each result's
   evidence, export a durable report, and accept signed offline updates.
3. Source files remain authoritative unless users explicitly choose otherwise.
4. Observation is read-only by default. Assistance is previewed; management is
   separately and narrowly authorized.
5. Every derived claim retains evidence, extractor version, observation time,
   confidence, and policy context.
6. Unknown and ambiguous are legitimate results. Gyst must not invent certainty
   to make a dashboard look complete.
7. Customer data does not leave its authorized boundary. Metadata-only,
   fingerprint-local, and air-gapped policies remain first-class.
8. Gyst evaluates work products and system state, not the productivity or worth
   of workers. Activity rankings and opaque employee scores are out of scope.
9. A user's artifacts and exported records remain usable if Gyst disappears.

An eventual release acceptance test should begin with a clean Windows machine,
documented installation media, and no network connection. The operator should be
able to install Gyst, scan a realistic project, inspect provenance, generate a
report, and verify a signed offline update.

## Open-source and sustainability model

Gyst uses the MIT License to minimize adoption and procurement friction. The
project accepts that organizations and vendors may use, modify, host, or build
commercial products around the software. The goal is useful public
infrastructure and broadly adopted contracts, not mandatory license revenue.

Sustainability may come from:

- Individual and organizational sponsorship.
- Paid engineering-system and release-readiness audits.
- Training, implementation, and integration work.
- Customer-funded connectors and extractors.
- Support for self-hosted and air-gapped deployments.
- Grants and institutional funding for open engineering infrastructure.

The software must remain useful without buying those services. The commercial
offer is help, judgment, implementation, and maintenance--not permission to run
the core tool.

## Reference contracts before platform breadth

The most durable outputs may be the public specifications and conformance tests
for:

- `Observation`
- `ArtifactRef`
- `Relation`
- `Finding`
- `GenerationRun`
- `ReleaseManifest`
- Connector and extractor capabilities
- Signed, inspectable transfer bundles

Gyst should provide a reference implementation and fixtures while allowing other
tools to produce and consume compatible records. Role-specific applications,
cloud connectors, planning systems, and federation follow demonstrated demand;
they do not precede a trustworthy narrow workflow.

## First field offer

Before treating Gyst as a software business, use it as the instrument behind a
repeatable **Release Readiness Audit**:

1. Scan agreed repositories and engineering folders read-only.
2. Map likely authorities, copies, exports, generated files, and unknowns.
3. Explain recent changes and Git provenance.
4. Identify stale, duplicate, ambiguous, or unreproducible artifacts.
5. Prepare a candidate manifest for one KiCad-centered release.
6. Export a human-readable report and conduct a findings review.

Early pilots validate the problem and improve the reference implementation. They
do not create proprietary core features or a mandatory hosted service.

## Validation gate

Broad product development pauses behind field evidence. A useful 45-day gate is:

- Five interviews involving real engineering-file and release problems.
- Two organizations permitting tests against realistic data.
- Two paid audits or equally strong funded-development commitments.
- One request to rerun the analysis on another release.
- One request for continuous observation or a new connector.
- Demonstrated time saved, release risk exposed, or handoff error prevented.

If those signals do not appear, Gyst remains valuable as public infrastructure,
an internal consulting tool, a teaching system, and a demonstration of systems
engineering judgment. It should not expand into a speculative PLM roadmap.

## Governance path

Operate as a foundation-shaped project before creating a legal foundation:

- Decisions, specifications, fixtures, security boundaries, and roadmaps are
  public.
- Maintainer authority and contribution review are explicit.
- Contributions require unambiguous authorship and compatible licensing.
- Project money, when it exists, is reported transparently.
- A fiscal host is preferred to premature incorporation.

Consider an independent legal entity only when the project has multiple active
maintainers, recurring organizational sponsors, material shared funds, contracts
requiring a neutral signatory, or trademarks/conformance marks requiring neutral
ownership. Governance exists to protect the public purpose, not to manufacture
administration.
