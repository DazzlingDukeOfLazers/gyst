# Product definition

## Thesis

Engineering work is rarely contained in one system. Source code may be in Git,
CAD on a file share, BOMs in spreadsheets, schedules in a kanban board, and shop
feedback in chat or email. Traditional PLM deployments often begin by asking the
organization to migrate and standardize. Gyst begins by observing what exists,
preserving where it came from, and connecting it incrementally.

The product is a **digital-thread integration layer**, not merely a Git client,
file sync product, project tracker, or PLM replacement.

## Product promises

Gyst should answer four questions reliably:

1. **What is this?** Identify an artifact, its project/product context, owner,
   lifecycle state, and relationships.
2. **Where is the authority?** Link to the canonical file or record and identify
   copies, exports, caches, and generated outputs.
3. **What changed?** Show human-meaningful revisions and their downstream impact,
   not only byte-level or Git-level changes.
4. **What needs attention?** Surface stale data, broken references, unreleased
   manufacturing inputs, schedule risk, cost movement, and unanswered feedback.

## Users and initial views

| Role | Decisions Gyst should support | Representative view |
|---|---|---|
| Executive / director | Are projects on schedule and budget? Where is risk concentrated? | Portfolio health, cost, labor, milestones, confidence |
| Engineering manager | Who owns work? Is data healthy? Is refactoring or release work accumulating? | Workload, change activity, hygiene, dependency risk |
| Engineer | Where is the current artifact? What changed recently? What is generated? | Project workspace, change feed, provenance, dependencies |
| Manufacturing engineer | What is released to build? What changed and why? How do I respond? | Release package, instructions, BOM diff, feedback loop |
| Floor lead / operator | What exact revision should be used for this work order? | Approved documents, tooling, redlines, acknowledgements |

The engineer/manager discovery workflow comes first. Executive aggregation is
only trustworthy after the underlying project, ownership, schedule, and cost
data have provenance and confidence.

## Core concepts

- **Organization / workspace**: an administrative and security boundary.
- **Project**: a bounded effort with owners, milestones, sources, and outputs.
- **Project membership**: an explicit or rule-derived many-to-many association;
  a project is not required to match a folder or repository boundary.
- **Product / assembly / part**: the physical or logical things being designed.
- **Artifact**: a file, folder, commit, sheet, row range, issue, message, release,
  or external record.
- **Source**: the authoritative system and locator for an artifact.
- **Observation**: an immutable statement about source state at a point in time.
- **Relation**: typed linkage such as contains, generated-from, supersedes,
  implements, blocks, built-from, or applies-to.
- **Revision / release**: a controlled, named product state. A Git commit alone
  is not automatically a manufacturing release.
- **Generation run**: a first-class record connecting exact inputs, tool and
  configuration versions, logs, and generated outputs.
- **Feedback**: a traceable report from a consumer back to an owning team.
- **Policy / finding**: a rule and its evidence, severity, status, and waiver.

## Scope boundaries

### In scope

- Discovery and inventory across local files, Git, and later cloud APIs.
- Content fingerprints, metadata, provenance, duplicate/copy detection, and
  change feeds.
- Explicit and inferred mapping of sources and artifacts to projects/products.
- Pluggable extractors for spreadsheets, BOMs, CAD metadata, and documents.
- Role-aware APIs and views over a normalized graph.
- Controlled release snapshots, comparisons, feedback, and acknowledgements.
- Policy-driven hygiene checks and safe, previewable remediation.
- MCP tools for querying Gyst and proposing bounded actions.

### Initially out of scope

- Becoming a general-purpose file sync provider.
- Transparent Git branching inside Dropbox, OneDrive, Google Drive, or similar
  synchronized directories.
- Full CAD/PDM locking semantics for every CAD vendor.
- Full ERP/MRP, procurement, payroll, or timekeeping functionality.
- Silent mutation, relocation, or deletion of user files.
- Treating heuristic inferences as authoritative business records.

## Experience model

Gyst has three progressively more invasive operating modes per source:

1. **Observe**: index metadata/content according to policy; never write back.
2. **Assist**: propose actions, previews, exports, and release packages; a human
   approves changes.
3. **Manage**: Gyst is explicitly selected as authority for a repository or
   artifact class and may perform policy-controlled writes.

Observation is the default. Mode is configured per source, not globally.

## Success measures

For a pilot team, Gyst should be able to demonstrate:

- Median time to locate the authoritative project artifact.
- Percentage of indexed artifacts with known project and owner.
- Percentage of released manufacturing artifacts with reproducible provenance.
- Time between engineering change and manufacturing acknowledgement.
- Duplicate/stale/generated-file findings resolved without data loss.
- Connector freshness and extraction failure rates.
- User-corrected inference rate (a guard against confident-but-wrong automation).

Avoid a vague single “project health” score until its inputs and uncertainty are
visible. Show evidence, freshness, and confidence beside every rollup.

## Initial persona and wedge

Start with a 5–30 person hardware/software engineering team whose project data is
split between Git and local/network folders, whose board designs use KiCad, and
whose BOM/release data is kept in CSV/XLSX. The wedge is: **find the right thing,
explain what changed, and prepare a trustworthy handoff**. This produces value
before replacing any incumbent tool.

## Product and part semantics

- A **product** is a sellable line item or product family.
- A **project** is any useful collection or subset of artifacts and work. Projects
  may overlap and may span repositories, folders, products, and organizations.
- An **item** has an internal identity and may have an internal part number.
- An **item revision** identifies a controlled physical design state. Integer
  revisions are the initial convention for physical items such as PCBs, but the
  scheme is configurable at the organization level.
- A **sourceable part** relates an item to manufacturer part numbers, approved
  alternates, and distributor offers. Distributor SKUs do not define item identity.
- A **release** selects exact item revisions, artifact versions, generated outputs,
  approvals, and a release manifest.

Generated files are not second-class attachments. They have artifact identities,
versions, ownership, retention, and `generated-from` relations. Gyst distinguishes
an output that was merely found from one that can be reproduced from a recorded
generation run.

## Manufacturing feedback families

Manufacturing feedback is not one generic comment type. Four initial workflows
share attachments, discussion, ownership, links, and audit history while keeping
distinct state machines:

| Workflow | Purpose | Typical terminal outcome |
|---|---|---|
| Comment / question | Clarification without controlled product disposition | Answered or closed |
| Nonconformance | Record that an observed result violates a requirement | Accepted, reworked, scrapped, or escalated |
| Deviation request | Seek approval to depart from released definition for a bounded scope | Approved/denied with scope and expiry |
| Change proposal | Request a durable change to product definition | Rejected or promoted into change control |

These are related records, not labels on a single mutable ticket. A nonconformance
may create a deviation and a change proposal without losing its own disposition.
