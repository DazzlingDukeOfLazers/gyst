# Round-two design

This design incorporates the first answered product and technical questions. It
turns them into behavioral contracts for the next prototypes. Where an answer
still depends on user testing, Gyst stores the choice as reversible policy rather
than baking it into artifact identity or history.

## 1. Primary vertical slice

The first complete workflow is Windows-first and KiCad-aware:

1. A user installs the Gyst agent and selects one or more local roots.
2. Gyst discovers folders, Git repositories, KiCad projects, PDFs, and tabular BOMs.
3. The user confirms suggested project membership and file-identity policies.
4. Gyst answers where the current artifact is, what changed, and which files are
   generated or suspected copies.
5. The user creates a PCB release candidate from exact source versions.
6. A generation run invokes a detected, supported `kicad-cli` to produce configured
   BOM, PDF, Gerber, drill, and placement outputs.
7. Gyst compares the candidate with the previous release and presents the semantic
   BOM diff, document/output diff, validation results, and provenance.
8. An authorized person approves an immutable release manifest.
9. Manufacturing reads the package and creates a question, nonconformance,
   deviation request, or change proposal against the exact released revision.

This is more useful than an inventory-only demo while remaining narrow enough to
validate the core model.

## 2. Project membership rather than repository boundaries

A project is a saved set over the artifact graph. It can contain a whole source,
selected subtrees, explicit artifacts, generated outputs, queries, and references
to other projects. An artifact can belong to zero, one, or many projects.

Membership evidence has a strict precedence order:

1. Explicit include/exclude assertion made by an authorized user.
2. A checked-in `.gyst/project.yaml` manifest.
3. A source-native marker such as a Git repository or KiCad project.
4. Organization rules based on paths, ownership, naming, or relations.
5. A Gyst suggestion based on content/dependency/history evidence.

Higher-precedence assertions override lower ones. User corrections become durable
assertions, so a subsequent scan or model update cannot silently undo them. At
scale, membership is evaluated incrementally from changed evidence rather than by
rescanning all projects.

Repository-boundary suggestions can use markers and dependency graphs, but Gyst
does not automatically split repositories, create monorepos, or equate repository
ownership with project membership.

## 3. Reversible file identity profiles

Gyst separates a logical `Artifact` from each observed `ArtifactVersion` and from
the physical `Location` where a version was seen. Identity profiles group observed
files without erasing the underlying observations.

| Profile | Example interpretation | Best for |
|---|---|---|
| Content/path exact | Every path is its own artifact; edits create versions | Stable source trees |
| Suffix-as-version | `widget_rev3.pdf` supersedes `widget_rev2.pdf` | Supplier revision drops |
| Suffix-as-identity | `connector_123.pdf` and `connector_124.pdf` are distinct | Part-number suffixes |
| Canonical-name | Incoming names become versions of `connector.pdf` | Controlled named deliverables |
| Compare-set | Similar files remain distinct but are offered side by side | Ambiguous subcontractor packages |

Profiles consist of scoped, ordered rules with named capture groups. Gyst previews
the grouping and explains each match before activation. A user can reclassify a
path or switch profiles; this rebuilds the derived grouping but never rewrites
observations, source files, releases, or audit history. Released manifests pin the
identity interpretation used when they were approved.

When confidence is low, Gyst chooses `Compare-set`. It never decides that a file
supersedes another solely from a numeric suffix.

## 4. Configuration and data-boundary files

Use a small, memorable configuration surface:

```text
.gyst/
  project.yaml        # project membership and metadata
  policy.yaml         # content, egress, extraction, retention, identity rules
  release.yaml        # release recipe and required approvals
.gystignore           # paths the agent must not inspect
```

Policy may also be set centrally for sources that cannot contain these files.
Effective policy is layered: server floor, organization, source, project, then
local exclusions. The most restrictive content/egress rule wins; a project cannot
relax an organization prohibition. Policy changes are validated, versioned, and
shown with an “effective policy” explanation.

Content handling has explicit levels:

- `exclude`: do not enumerate or retain the path.
- `metadata`: path-safe metadata only; no content read.
- `fingerprint`: locally hash content but do not upload it.
- `extract-local`: parse locally and send approved claims/previews.
- `content`: permit approved content storage on the configured server.

Egress is independently limited to `device`, `facility`, or `connected-server`.
The agent defaults to metadata and local fingerprints during onboarding and shows
what would leave the machine before enrollment is completed.

## 5. Authorization and information-flow safety

Connector credentials answer **what the connector can read**, not **who may see
the indexed result**. Conflating the two would allow a cross-project search or
derived relation to leak restricted filenames, customers, or part numbers.

The first authorization model has:

- Site administrator: operates the service; no automatic right to project content.
- Organization owner: manages members, sources, roles, and policy.
- Source administrator: enrolls a source and maps its principals/permissions.
- Schema steward: defines typed record schemas and controlled calculations.
- Project maintainer: owns membership, releases, and project policy within limits.
- Contributor: creates work and feedback according to workflow permissions.
- Viewer / manufacturing consumer: reads released or explicitly shared material.

Each observation carries a visibility label derived from source permissions and
policy. A derived claim or relation is visible only when its evidence is visible;
otherwise it is omitted or presented as a redacted dependency without leaking the
hidden endpoint. Cached search documents are authorization-scoped. Break-glass
access is explicit, time-bounded, and audited.

For a single-user local installation, these mechanics collapse to one principal.
They still exist in the data model so local data can later join a team server
without redefining every artifact.

## 6. Authoritative structured records, not better loose spreadsheets

Gyst should provide **Records**: typed, revisioned tables with friendly grid and
form views. Records preserve the accessibility of a spreadsheet while removing
the assumption that every user owns an independent mutable workbook.

A record schema can define:

- Stable row identity and uniqueness constraints.
- Typed fields: text, number/unit, money/currency, date, person, item, supplier,
  attachment, state, and relation.
- Required fields, allowed values, references, and validation rules.
- Controlled computed fields with versioned formulas and visible dependencies.
- Field- and workflow-level edit permissions.
- Draft, review, approval, release, and effective-date behavior.
- API, webhook, import, snapshot, CSV/XLSX export, and printable views.

Managers and directors may define organization schemas for cost, labor, and
schedule, but Gyst supplies safe primitives, migrations, and formula tests. Schema
changes are reviewed like code and do not reinterpret previously released records.

Existing workbooks remain valid source artifacts. Gyst imports them through a
mapping preview, retains workbook/sheet/cell provenance, and can monitor copies.
The target state is one authoritative record set with multiple filtered views,
not synchronized personal spreadsheet copies.

An early build/reuse spike should evaluate integrating a self-hosted structured
data product such as Grist behind an adapter. Gyst must still own artifact,
release, provenance, and workflow contracts so an external grid is replaceable.
Gyst authorization must not be delegated blindly to a grid implementation;
formula and schema-edit permissions in particular are part of the security review.

## 7. KiCad and PDF support

KiCad is the only native CAD/EDA target for the initial product. The extractor:

- Detects the installed CLI and records its exact version.
- Parses supported native project/schematic/board metadata read-only.
- Uses `kicad-cli` for supported validation and deterministic exports.
- Captures project variables, job/release recipe, input digests, output digests,
  logs, and environment metadata for every generation run.
- Produces BOM, PDF, Gerber, drill, placement, and other configured release outputs.
- Compares component fields, reference designators, DNP state, hierarchy, net and
  footprint assignments, placement summaries, and output inventories where the
  supported format exposes them.

Gyst initially compares KiCad designs but does not merge them. Visual PDF renders
and overlays are the fallback for unsupported CAD and a review aid for KiCad.
Support is declared by KiCad major version and tested against fixtures; an unknown
version may be indexed as files but cannot create an approved reproducible release.

## 8. Conflict and arbitrary-diff experience

A `Reconciliation` record contains base, Alice, Bob, and an output proposal. It is
created when concurrent candidates claim the same logical artifact version or an
authorized write would replace newer source state.

Resolution is semantic when possible:

- Text: block/line choices plus editable result.
- Typed records/BOM: row and field choices validated against schema.
- File sets: add/remove/rename decisions.
- PDF: page/render comparison; choose a candidate or retain both.
- KiCad and unknown binaries: compare known facts and renders, then choose one as
  current, retain both as candidates, or save one under a new identity.

The UI asks concrete questions (“Alice's value, Bob's value, or both as separate
candidates?”), always offers download/open-source actions, and never destroys the
unselected content. Finalization creates a new version with parents pointing to
the candidates and records every choice. There is no automatic binary merge in
the initial release.

## 9. Connected and air-gapped profiles

Both profiles run the same agent, core services, schemas, and clients.

### Connected

A user or IT administrator installs a server. Organization owners administer it
similarly to a Git forge. Edge agents and API connectors send policy-permitted
observations over mutually authenticated channels. No vendor cloud is required.

### Air-gapped

The server and agents operate without external identity, telemetry, update, model,
or license checks. Dependencies and updates are distributable as checksummed,
signed offline media. External exchange uses an inspectable **transfer bundle**:

- Signed manifest and schema versions.
- Selected releases, observations, records, relationships, and attachments.
- Explicit omissions/redactions and export-policy result.
- Content digests and originating organization/instance identifiers.
- Import preview, signature/trust result, and idempotency identifier.

Transfer is asynchronous replication, not live synchronization. Import never
silently gains write authority over the originating source.

## 10. Federation for an automotive-scale supply chain

Federation should exchange bounded claims rather than combine every company's
graph. Each organization keeps its own authority and namespace. Portable identity
uses an organization/instance identifier plus local stable ID; content uses strong
digests; releases and transfer bundles are signed.

An automaker could accept a supplier's signed release capsule containing approved
part/revision identity, interface documents, compliance evidence, and permitted
provenance while the supplier withholds its internal project graph. The automaker
can relate that capsule to vehicle programs, build lots, serial-number genealogy,
nonconformances, and service feedback. New supplier revisions arrive as new signed
claims, never as edits to previously accepted history.

This model supports connected federation and physical air-gap transfer with the
same envelope. Global part-number uniqueness is not assumed.

## 11. Search and scale gates

PostgreSQL remains the first metadata, relation, and search store. Use partitioned
observations, compact current-state projections, full-text/trigram indexes, and
authorization-aware query plans. Do not add a graph or search cluster merely
because the model contains relations.

Introduce a dedicated search index only when a representative load test shows,
after query/index tuning, one or more of:

- Interactive search p95 exceeds 500 ms at target concurrency.
- Faceting/fuzzy ranking requirements cannot be met without harming ingestion.
- Search indexes and write amplification materially degrade the transaction store.
- Independent search scaling or failure isolation is operationally necessary.

Introduce a specialized graph engine only when required multi-hop traversals miss
their latency target and a PostgreSQL implementation has been profiled, not guessed.
Keep observation storage and portable identifiers independent of either projection
so a large automotive deployment can replace or shard query infrastructure without
changing connector contracts.

## 12. Explicit prototype decisions

- Go is the implementation candidate for agent, server, and CLI because a single
  distributable service and Windows support are central; confirm it with scanning,
  filesystem-event, service-installation, and packaging spikes.
- TypeScript remains the web client candidate.
- A modular monolith plus edge agent and isolated extractors remains the starting
  topology.
- MCP begins read-only after the HTTP service contracts are stable.
- AI may suggest grouping, mapping, and summaries, but cannot define identity,
  release, access, or disposition without a recorded human/policy decision.

## Implementation references

- The official [KiCad command-line documentation](https://docs.kicad.org/10.0/en/cli/cli.pdf)
  defines the export surface to wrap and version-test rather than reimplement.
- The official [Grist REST API](https://support.getgrist.com/api/) and
  [access-rule documentation](https://support.getgrist.com/access-rules/) define
  the adapter/security questions for the structured-record spike.
