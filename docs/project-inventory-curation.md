# Project inventory, curation, and authority design

Status: design handoff for implementation and data-contract review

Primary reader: Claude, implementing the Go report and projection contracts

Primary users: engineers and engineering managers after a first real scan

Related record: [design-system.md](design-system.md), especially sections 1–5,
7–9, 12, and the open decision about the smallest useful project-detail view

## 1. Outcome

The first report successfully proves that Gyst can find meaningful structure in
a large, messy tree. The next view should help a person turn that evidence into
an understandable map of their work without making Gyst a file manager, project
tracker, or replacement source of truth.

The interaction loop is:

> Observe → explain → ask for judgment → record a reversible assertion →
> rebuild the projection → show what changed.

The immediate design goal is to make the current 32-project real scan useful.
The screen must:

1. Say which rows are declared projects and which are candidate boundaries.
2. Put ambiguous, nested, duplicate, and high-consequence candidates first.
3. Let a person simplify the view through filters, grouping, and saved local
   presentation state.
4. Summarize a selected project before exposing thousands of member files.
5. Show authority at the artifact level without pretending a project has one
   universal source of truth.
6. Prepare durable, explainable project-curation assertions without writing to
   source files.

This document extends the existing design record; it does not replace the
Projects/Places model or the read-only and evidence requirements.

## 2. What the first real scan taught

The real scan contains about 81,000 files, 32 projected projects, and 1,412 open
findings. The first HTML report in `internal/report/report.html` renders a flat
Projects table with columns for project, basis, confidence, file count, source
count, and member patterns. Selecting a project opens a drawer containing the
claim, evidence identifiers, member patterns, and up to 300 file links.

That is a successful contract reader and an unsuccessful interpretation tool.
Specific observations from the screenshot and report code:

- `godot` appears more than once, with the parent repository visible only in a
  clipped member path.
- Names such as `archive`, `mod`, and `firmware` are not meaningful without
  source and parent context.
- A manifest project and a marker suggestion receive the same row weight.
- Repeated `suggestion 0.60` labels communicate false precision without helping
  the user decide what to inspect.
- File count dominates the row even though `16,617 files` does not explain the
  project's purpose, composition, boundary, or authority.
- The drawer starts with audit-oriented identifiers and then becomes a raw file
  list. It does not answer “why should I care about this boundary?”
- Nested repositories and engine projects appear as siblings of their likely
  containing effort.
- The Findings view currently groups by the first path segment of the first
  subject rather than by actual project membership.
- Report metadata in the header is useful but visually competes with navigation
  and search.

The product should not hide this mess. The mishmash is evidence about how work is
organized. The design should make it legible and reviewable.

## 3. Product stance: curation, not CRUD

Generic create/read/update/delete controls are the wrong model for this screen.
Gyst does not own the repositories or folders it observes. A person should not
be able to rename, move, delete, or silently edit source artifacts from the
inventory.

The useful operations are append-only assertions over observed evidence:

- Confirm that a suggested boundary represents a useful project.
- Give a project a display alias without renaming its folder or repository.
- State that a candidate is part of another project.
- State that candidates in different places represent the same project.
- State that a suggested boundary should remain separate.
- Include or exclude an artifact or subtree from project membership.
- Mark a candidate as archived, reference material, generated output, vendored,
  or not useful as a project.
- Retract any previous assertion with actor, time, and reason preserved.

The UI verbs should be **Confirm**, **Describe**, **Relate**, **Include**,
**Exclude**, **Ignore as project**, and **Retract assertion**. Avoid **Edit
project** when the action actually adds a new interpretation.

Every operation must show:

- the observation or current projection being considered;
- the proposed assertion in plain language;
- the files or memberships whose projected presentation will change;
- what will not change, especially the user's source files;
- actor and reason;
- how to retract the assertion.

The first static report remains read-only. It may show disabled or explanatory
previews of these actions, but it must not imply that an HTML report can persist
them.

## 4. First-run information architecture

Keep the top-level navigation from the current report:

- Projects
- Places
- Findings

Add a first-run summary and review model inside Projects rather than adding a
fourth top-level destination.

### 4.1 Scan summary

The first Projects view should lead with a compact summary derived from the
report:

```text
32 project records from 80,947 observed files

2 declared by manifests       30 suggested from native markers
6 nested-boundary candidates   4 ambiguous authority sets
1,412 open findings            83% routine vendored/build content folded

[Review candidates] [Browse confirmed/declared] [View scan coverage]
```

Only show a number when the current report can calculate it honestly. For
example, nested-boundary counts require an explicit or safely derivable boundary
locator; do not infer them from project names.

Use **project records** in the total because a mixed list contains declarations
and suggestions. Use **candidate** for marker-based rows until a manifest or
human assertion declares them useful projects.

### 4.2 Review states

The Project list should offer these presentation filters:

- Needs review
- Declared or confirmed
- Ambiguous
- Nested boundaries
- Multiple places
- Has open findings
- No authority identified
- Ignored or archived, when durable assertions exist
- All project records

Filters are views over evidence and assertions, not project lifecycle fields
invented by the client.

The default after a first scan is **Needs review**. After a user has made durable
project assertions, remember the last local view.

### 4.3 Ordering

Order Needs review by expected consequence:

1. Conflicting declared authorities within the candidate's members.
2. Overlapping or nested boundaries.
3. Possible same-project identities across sources.
4. Candidates with high-severity findings.
5. Very large inferred boundaries.
6. Generic or duplicate display names.
7. Remaining marker suggestions.

Alphabetical order remains available. It is not the default review order.

## 5. Projects lens

### 5.1 Row design

Replace the current six-column implementation with a decision-oriented row:

| Column | Content |
|---|---|
| Project record | Display name, contextual source/root, and declared/candidate state |
| Boundary | Root or membership summary; nested/overlapping state when known |
| Places | Source count and source kinds |
| Attention | Findings and unresolved authority counts, kept separate |
| Composition | Owned/routine file summary rather than one undifferentiated count |
| Latest evidence | Age and independent scan coverage |

Do not display project membership confidence as a free-standing score column.
The meaningful label is **Declared by manifest** or **Suggested by native
marker**. The exact confidence belongs in the expanded explanation.

Generic names need context in the row:

```text
godot
inside 2Caves2Qud · personal-git

godot
inside raves-of-qud · personal-git
```

Do not rename either candidate automatically. A user may later assign an alias.

### 5.2 Grouping

Offer grouping by:

- Source/place.
- Parent boundary.
- Declared versus suggested.
- Review state.
- Source kind.

The parent-boundary view should be a restrained hierarchy, not a general graph:

```text
personal-git
├── 2Caves2Qud                       Suggested project
│   └── godot                        Possible nested project
├── raves-of-qud                     Suggested project
│   ├── godot                        Possible nested project
│   └── mod                          Possible nested project
├── get-in-loser                     Suggested project
└── job-search                       Suggested project
    └── archive                      Possible nested project
```

The label **Possible nested project** states a path relationship, not an
organizational conclusion. A nested repository may be a real independent
project.

### 5.3 Selection continuity

Selecting a project and switching to Places should:

- keep the selected project;
- expand every source branch containing a member;
- dim unrelated branches rather than remove them;
- show the number of matching files in each contributing branch;
- preserve search and filters.

This is already a recorded design requirement and should become the primary way
to understand where a cross-source project lives.

## 6. Project workspace and drawer

The current project drawer should become an overview, not a file dump. It can
remain a drawer for the static report, but its content should anticipate a later
full project workspace.

### 6.1 Overview order

1. **Identity** — name, contextual location, record state, and project ID behind
   technical details.
2. **Why Gyst included it** — one plain-language explanation, evidence count,
   and **Why?**.
3. **Boundary** — sources, roots or patterns, overlaps, nested candidates, and
   excluded or policy-limited areas.
4. **Attention** — grouped findings, authority ambiguity, incomplete scans, and
   unassigned members.
5. **Composition** — first-party, vendored/build, placeholder, content-withheld,
   and unknown counts.
6. **Authority within this project** — declared, likely, multiple, and none
   counts with links to the affected artifacts.
7. **Relationships** — parent/child assertions, possible same-project records,
   generated-from relations, and cross-project duplicates.
8. **Files** — folder-first summary, search within project, then individual
   files on demand.
9. **Evidence and assertion history** — IDs, actors, reasons, timestamps, and raw
   technical details.

### 6.2 Compact wireframe

```text
┌─ get-in-loser ─────────────────────────────────────────────┐
│ Candidate project · Suggested by native marker             │
│ inside personal-git                                        │
│                                                            │
│ Why this appears                                           │
│ Git and Node markers suggest a boundary. No manifest or     │
│ human confirmation declares one.                   [Why?]  │
│                                                            │
│ Boundary                                                   │
│ personal-git:get-in-loser/** · 1 place · 16,617 files      │
│ 14,902 routine/vendored folded · scan complete             │
│                                                            │
│ Needs attention                                            │
│ 2 authority conflicts · 18 duplicate groups · 1 nested     │
│ candidate                                                   │
│                                                            │
│ Authority among members                                    │
│ Declared 3 · Likely 8 · Multiple 24 · None 1,680           │
│                                                            │
│ [Review boundary] [View in Places] [Search project files]  │
│                                                            │
│ Overview | Boundary | Authority | Relations | Files | Audit │
└────────────────────────────────────────────────────────────┘
```

Numbers above are illustrative layout copy, not fixture values. The
implementation must derive every displayed count from the report.

### 6.3 File disclosure

Do not show 300 file links on opening the project. Show:

- top-level folders;
- file-kind or extension composition where it is a direct presentation
  derivation;
- routine/vendored content folded;
- files with findings or authority ambiguity first;
- a search-within-project field;
- an explicit action to browse all member files.

Internal IDs such as `mark_…` and `obs_…` remain copyable in Audit or Evidence.
They should not be the second line a normal user reads.

## 7. Authority and “source of truth”

The current code correctly resolves authority for a file and its peers. It does
not establish one authority for an entire project. The UI must keep that
distinction.

A project may have different authorities for:

| Concern | Example authority |
|---|---|
| Working source | A repository and branch |
| Reviewed source | A merged default branch or approved change |
| Released behavior | A signed release or shipped binary |
| Documentation | A manual, website source, or controlled PDF |
| Hardware design | A KiCad project or approved item revision |
| Manufacturing package | A released bundle or PLM record |
| Project status | A maintainer, milestone record, or tracker |

The research and product question is:

> Authoritative for what, at which lifecycle state, according to whom, and
> supported by which evidence?

### 7.1 What can ship with the current contract

For a selected project, aggregate its member files by the existing
`files[].authority.state` values:

- Declared authority.
- Likely authority.
- Multiple authority candidates.
- No authority identified.

Clicking a count filters the member files and opens each file's existing
authority explanation. Label the section **Authority among project files**, not
**Project source of truth**.

### 7.2 Future scoped authority

Do not infer source-code, documentation, release, or manufacturing roles from
extensions or paths alone. A future contract should allow a human or accepted
manifest to declare the concern, scope, lifecycle state, authority target,
evidence, actor, reason, and effective time.

That is a product-model extension and needs a separate decision before code. It
must coexist with current file-peer authority rather than replacing it.

## 8. Findings in project context

The current HTML groups a finding using the first path segment of its first
subject. That worked as a first page but is not project semantics.

The project workspace should group findings by:

1. Actual project memberships of all visible subjects.
2. Finding rule.
3. Severity and status.

A cross-project duplicate belongs visibly to every implicated project and to a
cross-project group. It should not be assigned solely to whichever subject was
listed first.

Within a project, summarize:

- open findings by rule;
- findings that cross project boundaries;
- findings whose evidence is restricted or policy-limited;
- acknowledged and waived findings separately from open ones.

Severity remains distinct from confidence, and a finding count is not a project
health score.

## 9. What the existing report contract supports

| Design need | Current contract | Implementation note |
|---|---|---|
| Declared versus suggested project | `projects[].basis`, `confidence`, `explanation`, `evidence` | Use words first; remove free-standing numeric confidence column |
| Member sources and patterns | `projects[].members[]`, `source_ids` | Show exact patterns under Boundary |
| Member files | `files[].projects[]` | Build an index once; do not repeatedly scan the full array per render |
| Composition | `files[].vendored`, `placeholder`, `content_level`, `present` | Derive counts; label only directly supported categories |
| File authority | `files[].authority` | Aggregate states within the selected project |
| Findings | `findings[].subjects`, evidence, rule, severity, status | Join visible file subjects to their memberships |
| Source context | `sources[]` location, root, pass, freshness, coverage | Keep age and coverage separate |
| Identity/grouping | `files[].grouping`, `artifacts[]` | Never present `is_current` as declared authority |
| Audit history | `assertions[]` | Current assertions are file authority only |
| Cross-source relations | `relations[]` | Use focused relation lists before a graph |

The first redesign can therefore ship substantial filtering, summarization,
selection continuity, and project-context findings without changing a schema.

## 10. Data-contract work needed from Claude

The following needs are not present in the report contract and must not be
invented in the client.

### 10.1 Durable project-curation assertions

The current assertion implementation addresses file authority. The design needs
a code-side proposal for append-only, retractable assertions covering:

- candidate confirmed as a project;
- candidate ignored as a project;
- display alias or description;
- project contains/is part of another project;
- two projected candidates represent the same project;
- explicit membership include and exclude.

The contract must retain actor, reason, time, evidence, retraction, and
precedence. It must not mutate marker or manifest observations.

### 10.2 Explicit boundary locators

The UI needs a safe way to show that `raves-of-qud/godot` is physically nested
inside `raves-of-qud` without parsing a glob pattern and treating that parse as
truth. The report needs either an explicit marker/boundary locator or another
documented way to derive it from evidence.

Physical nesting is not project containment. The contract should make the
former visible without manufacturing the latter.

### 10.3 Finding-to-project projection

The client can join file subjects to `files[].projects[]` in the current static
report, but the intended semantics for subjects that are not files, hidden
evidence, and very large reports need a code-side decision. A projected list of
implicated project IDs may be appropriate if it remains evidence-derived and
authorization-safe.

### 10.4 Observation contents in evidence views

The report cites observation IDs but does not include the observations. The
drawer cannot yet show what a cited observation actually said. Decide whether
the portable report embeds a visibility-filtered observation summary or whether
evidence detail remains a separate local query in an interactive client.

### 10.5 Scoped authority is deferred

Do not add project-level “source of truth” fields in this implementation round.
First document the distinction between file-peer authority and concern-scoped
authority, then create a product/architecture decision if interviews show a
stable vocabulary.

## 11. Interaction details

### Search

- Match project name, contextual parent/root, ID, member pattern, and file path.
- State why a row matched.
- Support quoted exact paths.
- Search within a selected project without losing the global query.

### Filters

- Filters are additive chips with a visible clear-all action.
- Filter state survives switching between Projects and Places.
- Counts say whether they refer to all report data or the current filtered set.
- Saved filters remain local and require no account.

### Sorting and grouping

- Every sortable header is a real button with current direction announced.
- Group expansion state is keyboard accessible.
- Grouping does not change project identity or membership.

### Drawer and focus

- Selecting a row produces a visible selected state in the table.
- Opening the drawer moves focus to its heading or first meaningful control.
- Escape closes the drawer and restores focus to the originating row.
- Drawer sections use headings and landmarks; tabs, if used, follow ARIA tab
  behavior.
- A drawer link can deep-link to the selected record when the report gains URL
  state.

### Large reports

- Build file, project, source, finding, and authority indexes once after parsing.
- Do not filter the entire 80,000-file array once for every row render.
- Render summaries first and window long file lists.
- Consider excluding routine vendored detail from the single HTML file before
  adding more client complexity; preserve its aggregate count and export path.

## 12. Ethnographic research plan

Do not ask users abstractly what their source of truth is. Give them a real task
and observe how they establish authority.

### Sessions

Use one engineer and, where possible, the person receiving their release. Ask:

1. Find the file you would edit for a named change.
2. Explain how you know it is the right file.
3. Find the version that was actually released or manufactured.
4. Explain why another apparent copy exists.
5. Show what you do when two people disagree about which copy is current.
6. Prepare a release or handoff while narrating each authority decision.
7. Identify a folder Gyst calls a project that you would not call a project.
8. Identify one effort that spans several folders or repositories.

### Watch for

- Local vocabulary: project, product, job, board, release, archive, customer,
  branch, working copy.
- Authority performed socially: “ask Pat,” a review meeting, a signed PDF, a
  default branch, a manufacturing traveler.
- Different authorities at working, reviewed, released, and as-built stages.
- Copies that serve legitimate roles: backup, fork, vendor drop, release
  snapshot, export, cache, or handoff.
- Shadow systems and offline paths: email attachments, USB media, shared-drive
  conventions, desktop folders, and vendor portals.
- Missing knowledgeable people and how confidence changes in their absence.
- Sensitive paths or names that should not appear to another role.
- Whether an apparent nested project is an independent deliverable, a component,
  an engine checkout, or merely tool output.

Record misunderstandings, observed decision steps, and words users already use.
Do not convert individual habits into universal model fields after one session.

## 13. Privacy and power review

The real scan includes personal and potentially sensitive roots such as a job
search beside engineering projects. Before using this report across a team:

- make scan scope and report visibility conspicuous;
- support exclusion before collection through `.gystignore` and effective
  policy;
- avoid leaking restricted filenames through counts or derived relations;
- consider path redaction or audience-specific path presentation;
- show what was omitted, without revealing the omitted content;
- never turn project/file activity into a person-level ranking;
- treat source ownership as routing and authority evidence, not productivity.

The static report's existing visibility warning remains necessary but is not a
substitute for an export preview.

## 14. Implementation sequence

### Stage A — current contract, static report

1. Add the Projects scan summary.
2. Replace numeric confidence column with declared/candidate language.
3. Add sort, filter, grouping, and a Needs review view.
4. Build indexes once and derive project composition, authority, and finding
   summaries.
5. Replace project file dump with the overview order in section 6.
6. Preserve selection when switching Projects and Places.
7. Group findings using actual file memberships.
8. Add keyboard and focus behavior, URL state, and print summary.

### Stage B — contract additions

1. Expose explicit boundary locators.
2. Define and implement project-curation assertions.
3. Project finding-to-project relationships if client derivation is inadequate.
4. Add visibility-filtered observation detail or a documented local evidence
   query.

### Stage C — interactive application

1. Present assertion previews and persist them through ordinary application
   services.
2. Show assertion/retraction history in the project Audit view.
3. Add saved local views.
4. Validate concern-scoped authority language through field research before
   extending the model.

## 15. Acceptance criteria

The redesigned inventory is successful when:

- a user immediately understands that marker rows are candidates, not confirmed
  organizational projects;
- two `godot` candidates are distinguishable without opening either drawer;
- a user can isolate declared projects, suggestions, multi-source projects,
  nested boundaries, and projects with findings;
- opening a 16,000-file candidate yields an explanation and summary rather than
  hundreds of links;
- selecting a project and switching to Places reveals every contributing
  location;
- findings are associated through membership rather than first-path heuristics;
- authority is shown for artifact peers and never collapsed into one project
  source-of-truth badge;
- no control implies that Gyst will rename, move, or delete source files;
- all future curation operations are described as append-only, evidence-backed,
  retractable assertions;
- the user can distinguish observed facts, declarations, suggestions, unknowns,
  policy limits, and errors without relying on color;
- a keyboard user can sort, filter, group, select, inspect, switch lenses, and
  return from the drawer without losing position;
- the report remains useful offline and legible at 100–200% Windows scaling.

## 16. Explicit non-goals

- File-manager CRUD.
- Automatic project merging or splitting.
- Automatic repository reorganization.
- A universal project health score.
- One source-of-truth badge for an entire project.
- Inferring organizational ownership from Git activity.
- A global relationship graph as the default navigation.
- Treating the newest file, default branch, or sole observed copy as authority.
- Turning the static report into an ungoverned write client.
