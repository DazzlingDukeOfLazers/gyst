# System architecture

## Architectural stance

Gyst separates **source authority**, **observed facts**, and **derived views**.
Connectors observe authoritative systems. An append-only event log records what
was seen. Projectors build query models and a relationship graph. User interfaces
and agents use those models but always retain links back to evidence.

```text
 Local agent       Git providers       Cloud/storage APIs       Work systems
     |                  |                       |                     |
     +------------------+---- connectors -------+---------------------+
                               |
                        observations/events
                               |
              +----------------+----------------+
              |                                 |
       metadata/catalog                 content/extractors
              |                                 |
              +-------- provenance graph -------+
                               |
                API / search / subscriptions / MCP
                               |
                  web UI / CLI / role-specific clients
```

## Components

### Edge agent

A small headless daemon runs near data that cannot or should not be uploaded
directly. It discovers configured roots, respects ignore and policy rules,
fingerprints content, invokes sandboxed extractors, and sends observations to the
core. It maintains a local queue for offline operation.

The agent must:

- Default to read-only and never cross configured roots.
- Avoid following symlinks outside policy boundaries.
- Detect file stability before hashing or parsing a file being written.
- Treat network and synced folders as sources, not working Git repositories.
- Redact or omit content according to classification policy.
- Make CPU, I/O, bandwidth, and schedule limits configurable.
- Persist its cursor so scans are resumable and idempotent.
- Run as a first-class Windows service with an understandable per-user mode for
  sources that depend on the interactive user's credentials.

For opt-in managed local folders, Git may be kept in a private mirror/worktree
outside the user-visible or cloud-synced directory. The visible folder is imported
and exported through a journaled reconciliation process; `.git` is never placed
inside a directory controlled by another sync engine.

### Connectors

Each connector implements a capability-oriented contract rather than pretending
all sources are filesystems:

```text
discover(cursor) -> observations, next_cursor
fetch(locator, version) -> bytes/stream       [optional]
subscribe(checkpoint) -> events               [optional]
propose(change) -> preview                     [optional]
apply(approved_change, idempotency_key) -> result [optional]
```

Connectors declare consistency, versioning, write, webhook, rate-limit, and
content-access capabilities. Cloud drives must use provider APIs and stable item
identifiers rather than watching locally synchronized replicas.

### Event and metadata core

Start with PostgreSQL for tenant/configuration data, append-only observations,
relationships, and query projections. Use its full-text and JSON capabilities
before adding a separate graph/search database. Binary content is optional and
goes to an S3-compatible content-addressed store when policy permits.

Key invariants:

- Observations are immutable; corrections create new observations.
- A source locator plus native version identifies source state.
- Processing is at-least-once and projectors are idempotent.
- Derived entities cite the observations that support them.
- Tenant and source authorization is enforced before search/index filtering.
- Deletion/tombstone and retention semantics are explicit per connector.
- Logical identity/grouping is a rebuildable projection; immutable observations
  retain paths, versions, and the identity-policy version used at interpretation.
- Generated outputs cite a generation run containing exact inputs, tool versions,
  configuration, logs, and output digests.

### Extractors

Extractors turn content into typed claims and previews. Examples include file
type detection, CSV/XLSX schema inference, BOM extraction, PDF text, CAD property
and dependency extraction, and generated-file classification. Run untrusted
parsers out of process with resource limits and no network by default.

An extractor result includes its name/version, input digest, output schema,
warnings, and confidence. Re-running a new extractor version must not rewrite
historical results.

### API and clients

The headless server exposes a versioned HTTP API (REST initially), an event stream,
and OpenAPI schema. A CLI supports administration, discovery, debugging, and
automation. The web UI is a separate client and should not contain business logic
that other clients need.

MCP is an adapter over the same application services, not a privileged back door.
Initial tools should be read-only (`find_artifact`, `recent_changes`,
`explain_provenance`, `compare_releases`, `list_findings`). Mutating tools require
preview, explicit authorization, idempotency keys, and auditable results.

### Structured records

A typed record service provides authoritative, revisioned grids and forms for
BOM, cost, labor, schedule, and organization-defined data. It exposes stable row
identities, validation, references, controlled formulas, workflows, and snapshots.
Spreadsheet import/export are boundaries, not the internal consistency model.

## Storage strategies

| Source | Default strategy | Avoid |
|---|---|---|
| Existing Git | Observe/fetch native repository and provider events | Rewriting history or forcing a monorepo |
| Unsynced local folder | Agent inventory; optional private Git mirror | Surprise commits or automatic moves |
| Network share | Scheduled, rate-limited agent inventory near the share | Assuming atomic events or reliable watches |
| Cloud/synced drive | Provider API, webhook plus reconciliation scan | Git metadata in the synchronized directory |
| Large/binary engineering files | Content hashes and native versions; optional object store | Putting all binaries directly in Git history |
| Spreadsheet | Preserve workbook as artifact; extract typed tables with cell provenance | Treating an inferred schema as source truth |

Git LFS or git-annex can be supported for repositories that already use them, but
neither should be imposed as Gyst's universal storage model.

## Build versus reuse

### Recommended

- Integrate with Gitea/Forgejo/GitHub/GitLab through their APIs as Git providers.
- Use PostgreSQL, an S3-compatible object store, OpenTelemetry, and standard
  identity protocols (OIDC/SAML via an identity provider).
- Reuse mature parsers and conversion tools behind the extractor boundary.
- Study Nextcloud/WebDAV semantics and support them as sources where useful.

### Do not fork yet

Forking Gitea would couple the product to a software-forge domain model and make
upstream maintenance a permanent cost. Gyst's differentiator is cross-system
provenance and product/manufacturing context. Build it as a separate service and
integrate first. Reconsider a fork only if a validated user workflow requires a
deeply unified Git-hosting experience that APIs or extensions cannot provide.

Likewise, do not begin by forking a Dropbox-like sync engine. Gyst needs change
observation and optional managed storage, not a second general sync protocol.

## Deployment profiles

1. **Solo/local**: agent, embedded queue, local API/UI; useful without a server.
2. **Team/self-hosted**: central server, PostgreSQL, object store, identity, and
   agents/connectors.
3. **Air-gapped**: the team profile with no external runtime dependencies and
   signed, inspectable transfer bundles for asynchronous import/export.
4. **Federated** (later): independently administered Gyst instances exchange
   selected, signed release and provenance records.

Keep the component boundaries consistent across profiles. SQLite may support the
solo profile, but PostgreSQL remains the reference behavior.

## Security and trust

- Mutual authentication between edge agents and server; short-lived credentials.
- Per-source service identities and least-privilege scopes.
- Encryption in transit and at rest; secrets stored outside ordinary config.
- Content classification and no-content/metadata-only indexing modes.
- Tenant isolation and authorization applied at ingestion and query time.
- Tamper-evident audit records for release, approval, write-back, and waiver.
- Signed agents/releases and a documented threat model before production pilots.
- Never send proprietary content to an external model without explicit policy.
- Treat connector read authority and indexed-result visibility as separate checks.
- Filter derived relations and search documents by the visibility of their source
  evidence so hidden metadata cannot leak through the graph.

## Suggested implementation shape

Delay a language/framework commitment until the first walking-skeleton spike.
The core needs strong concurrency, filesystem, API, and cross-platform support.
Go is the leading candidate for server/agent/CLI distribution; TypeScript is a
likely web-client choice. Extractors may use isolated Python processes where its
document/data ecosystem materially helps. Contracts matter more than a monorepo
language mandate.
