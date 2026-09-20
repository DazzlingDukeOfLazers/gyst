# Gyst design system and first-view plan

Status: proposed direction for the pre-alpha static report  
Primary users: engineers and engineering managers  
Platform: Windows-first, offline, read-only by default  
Source basis: [designer-brief.md](designer-brief.md), product definition, round-two design, decisions, rename notes, v0 schemas, current website, and icon map

## 1. Design thesis

Gyst should feel like an evidence workbench: dense, calm, explicit, and inspectable. It should not look like a file manager with extra badges, a graph explorer, or a project-management dashboard.

The first experience should use two synchronized lenses over the same data:

1. **Projects** — the default working view once discovery has produced useful groupings. It answers “what work belongs together?” across repositories, folders, and shares.
2. **Places** — the physical source tree. It answers “where was this observed?” and is the natural starting point during discovery.

Switching lenses must preserve search, selection, and filters. Selecting a project in Projects highlights every contributing source in Places; selecting a folder in Places shows every project that includes it. This makes the difference between logical membership and physical location visible instead of explaining it only in documentation.

The interface must make five distinctions unmistakable:

- observed fact versus derived claim;
- declared authority versus inferred authority;
- unknown versus policy-withheld versus failed;
- confidence versus severity;
- freshness versus scan completeness.

## 2. Product design principles

### Evidence before assertion

Every derived claim has a visible “Why?” affordance. The UI never states a relationship more strongly than its evidence permits.

### Location is evidence, not identity

Paths explain where bytes were observed. They do not define a project, logical artifact, revision, or release.

### Honest uncertainty is a successful state

“Unknown,” “multiple candidates,” and “not enough evidence” are designed outcomes, not empty or broken UI. Gyst earns trust by refusing to invent certainty.

### State is composable, not collapsed

Do not combine confidence, freshness, completeness, severity, and policy into a single score. Show the small number of states relevant to the current decision.

### Progressive disclosure protects meaning

Start with projects, sources, claims, and attention items. Reveal files, observations, extractor details, and raw JSON only on demand.

### Read-only means visibly read-only

Use verbs such as **Review proposal**, **Preview**, **Open source**, and **Export**. Never imply that selecting a suggestion will immediately move, rename, delete, or rewrite a file.

### The interface observes work, not workers

People appear only where ownership, authorship, or approval is relevant to the artifact. No activity rankings, productivity comparisons, or person-level health signals.

## 3. Information architecture

### First release navigation

- **Discover**
  - Projects lens
  - Places lens
- **Changes**
- **Findings**

For the first shipped report, Discover is the complete experience. Changes and Findings may begin as filtered views linked from counts rather than fully independent destinations.

### Discover header

The header contains:

- report title and observation window;
- last completed scan time;
- search across project names, paths, artifact names, and identifiers;
- Projects / Places segmented control;
- filters for source kind, attention state, freshness, membership basis, and content policy;
- density control: Comfortable / Compact;
- **About this report** with schema version, generator version, generation time, and offline status.

Do not show a global “health” score.

### Projects lens

Use a sortable table/list, not cards. Recommended columns:

| Column | Purpose |
|---|---|
| Project | Name, optional description, and membership state |
| Sources | Count plus source-kind icons; expand to exact roots |
| Authority | Declared, inferred, ambiguous, or not identified |
| Attention | Open findings by severity, without a combined score |
| Latest evidence | Most recent relevant observation, with scan completeness separate |
| Membership | Explicit, manifest, native marker, organization rule, suggestion, or mixed |

Project rows expand inline to show contributing sources and a short artifact summary. Opening the project moves to a project workspace only after the first-view prototype proves that the overview model is understandable.

### Places lens

Use a source-grouped tree with one root per enrolled source, never one artificial universal filesystem.

Each source root shows:

- source kind and exact root (`C:\`, `D:\Engineering`, `\\server\share`, repository, or cloud-synced root);
- last observation time;
- pass result: complete, partial, interrupted, unavailable, or never scanned;
- effective content level;
- number of contributing projects and open findings.

Each folder or artifact can carry zero, one, or several project-membership chips. Multiple chips are expected and must not look like a conflict. Unassigned artifacts use a neutral “No project membership” label, not an error state.

The tree provides physical orientation, but the adjacent detail pane explains logical context. A project breadcrumb must never be constructed from folder ancestry.

### Recommended desktop layout

```text
+------------------------------------------------------------------------------+
| GYST  Discover   Changes   Findings        Search...        Report details    |
+------------------------------------------------------------------------------+
| PROJECTS | PLACES     Filters: Attention  Source kind  Freshness   Density    |
+----------------------+-------------------------------------------------------+
| Saved filters        | Main table or source tree                             |
|                      |                                                       |
| All                  |  Project / source rows                                |
| Needs attention      |  expandable children                                 |
| Ambiguous            |                                                       |
| Policy limited       |                                                       |
| Stale sources        |                                                       |
+----------------------+-----------------------------------+-------------------+
| Selection and filters remain in place                     | Evidence drawer   |
|                                                           | opens on demand   |
+-----------------------------------------------------------+-------------------+
```

At widths below 1100 px, the saved-filter rail collapses. The evidence drawer becomes an overlay. The first release is optimized for desktop Windows; mobile is readable but not a primary workflow.

## 4. The project-to-place interaction

The core transition is a lens change, not a navigation jump.

### From Places to Projects

1. A source tree begins with discovered physical roots.
2. Project membership chips overlay folders and artifacts.
3. Selecting a membership chip switches to Projects with that project focused.
4. The project row expands to list all contributing roots, including roots outside the original tree branch.

### From Projects to Places

1. Select a project row.
2. Switch to Places.
3. Every contributing source root remains visible; non-contributing branches are dimmed, not removed.
4. A summary states, for example, “3 sources · 2 repositories · 1 network share.”

This interaction is the simplest way to teach that projects cross folders without introducing a graph. A graph should be deferred until users need to answer multi-hop relationship questions that lists, tables, and focused relation diagrams cannot answer.

## 5. Semantic state language

### Confidence

Confidence belongs to a specific claim, relation, or membership assertion—not to a project, person, source, or entire report.

Use words first and expose the exact number nearby in expanded/detail contexts:

| Data value | Display label | Treatment | Meaning |
|---|---|---|---|
| exactly `1.0` | Observed | solid neutral badge; filled dot | Directly observed fact only |
| `0.80–0.99` | Strong inference | solid border; three filled ticks | Supported inference; still not a fact |
| `0.40–0.79` | Tentative | dashed border; two filled ticks | Plausible, needs review |
| `0.01–0.39` | Weak candidate | dotted border; one filled tick | Useful for compare-set, not selection |
| no support / no score | Unknown | outline diamond with `?` | Evidence cannot answer |

These bands are presentation policy and must be validated against real output. The exact numeric value remains available in the evidence drawer and in exported data. Do not use red/amber/green for confidence; confidence is not severity or approval.

Examples:

- `Observed` — file metadata read directly from a source.
- `Strong inference · 0.95` — one-to-one content match supporting `renamed-from`.
- `Weak candidate · 0.30` — one item in an ambiguous compare set.
- `Unknown` — no evidence supports choosing one candidate.

### Authority language

Use these exact levels:

- **Declared authority** — supported by an explicit assertion or accepted manifest.
- **Likely authority** — inferred, with confidence and “Why?” visible.
- **Multiple authority candidates** — no basis to choose.
- **No authority identified** — the evidence does not contain an answer.

Never label a low-confidence candidate “authoritative.”

### Unknown, withheld, error, and unavailable

| State | Meaning | Visual | Example copy |
|---|---|---|---|
| Unknown | Gyst has insufficient evidence | gray outlined `?`; no warning color | “Current version unknown” |
| Withheld by policy | The system deliberately did not read or retain allowed fields | blue lock/shield; stable info treatment | “Content not read · metadata policy” |
| Error | An attempted operation failed | red error icon; explicit next step | “Extractor failed · view details” |
| Unavailable | The source could not be reached at observation time | orange disconnected-source icon | “Share unavailable during scan” |
| Not observed yet | No attempt has completed | hollow clock | “Waiting for first scan” |
| Redacted | Evidence exists but the viewer cannot see it | solid shield; avoid revealing endpoint metadata | “Restricted evidence” |

Never use a blank cell or em dash for these states. A blank cell is reserved for “not applicable.”

### Freshness and completeness

Freshness is two values, not one badge:

1. **Age:** “Seen 4 min ago,” “Seen 3 days ago.”
2. **Coverage:** complete, partial, interrupted, unavailable, or unknown.

Fresh/stale thresholds are source-policy-specific. A local folder may become stale in minutes while a weekly network-share scan can still be current after several days. Display the expected cadence in the tooltip or evidence drawer: “Current for this source · expected every 7 days.”

Recommended states:

- Current — within expected cadence.
- Due soon — inside the final 20% of the expected interval.
- Stale — cadence exceeded.
- Interrupted — pass began but did not complete.
- Unavailable — source could not be reached.
- Never scanned — no completed pass exists.

An interrupted scan can be recent and incomplete. Show both facts: `Seen 8 min ago` + `Interrupted`.

### Findings and severity

Finding severity uses color and icon together:

- Info — blue circle/info.
- Low — neutral outlined flag.
- Medium — orange triangle.
- High — red diamond/exclamation.

Finding status (`Open`, `Acknowledged`, `Waived`, `Resolved`) is a separate text chip. A waiver always exposes who waived it, why, when, and any expiration.

## 6. Visual foundation

### Relationship to the website

Retain the website’s warm paper, near-black ink, electric blue, orange, and acid highlight so the report clearly belongs to Gyst. Adapt the visual grammar from “poster” to “workbench”:

- reduce 2 px rules to 1 px for dense tables;
- reserve offset shadows for dialogs and major overlays;
- use the acid color for focus/selection, not status;
- use orange and red only when something needs attention;
- preserve strong typography and literal iconography.

### Color tokens

| Token | Value | Use |
|---|---:|---|
| `canvas` | `#F2EDDF` | application background |
| `surface` | `#FFFDF6` | tables, drawers, panels |
| `surface-muted` | `#E7E1D3` | selected sub-rows, grouped headers |
| `ink` | `#11110F` | primary text and strong borders |
| `ink-muted` | `#5F5B51` | secondary text |
| `rule` | `#C8C1B2` | grid and divider lines |
| `blue` | `#3157FF` | links, focus, policy information |
| `orange` | `#C84424` | medium attention and source interruption |
| `acid` | `#D8FF43` | current selection and keyboard highlight |
| `red` | `#B42318` | failed operations and high severity |
| `green` | `#206A4A` | completed/verified state only |

The marketing site’s brighter `#FF5C35` may remain decorative. Use the darker application orange for readable text and icons. All text/background combinations must pass WCAG 2.2 AA; status meaning must also be conveyed by icon, label, and border/pattern.

### Typography

Use installed system fonts only:

- UI and prose: `"Segoe UI Variable", "Segoe UI", system-ui, sans-serif`.
- paths, digests, observation IDs, versions: `"Cascadia Mono", Consolas, monospace`.

Type scale:

| Role | Size / line | Weight |
|---|---|---|
| Page title | 28 / 36 | 700 |
| Section title | 20 / 28 | 700 |
| Row title | 14 / 20 | 600 |
| Body | 14 / 20 | 400 |
| Compact row | 13 / 18 | 400 |
| Metadata | 12 / 16 | 500 |
| Code/path | 12 / 18 | 400 |

Do not uppercase long labels. Uppercase is limited to short metadata tags and column headings.

### Spacing, sizing, and shape

- Base spacing unit: 4 px.
- Common gaps: 8, 12, 16, 24, 32 px.
- Comfortable row: 44 px minimum.
- Compact row: 32 px minimum.
- Icon sizes: 16 px inline, 20 px controls, 24 px empty states.
- Border radius: 2 px for controls, 0 px for table groupings, 4 px for overlays.
- Divider: 1 px neutral; 2 px ink only for selected/high-priority boundaries.
- Focus ring: 3 px blue with 2 px offset; acid interior highlight may supplement it.

### Iconography

- Reuse the existing locally bundled literal icons for high-level nouns such as document search, revision, release, server, PCB, CAD, and BOM.
- Add a small consistent 16/20 px status set for observed, inferred, unknown, policy, error, stale, partial scan, and restricted evidence.
- Prefer simple stroked or filled geometry that remains distinguishable at 100% and 200% Windows scaling.
- Bundle every asset locally and preserve creator, source, and license metadata.
- Never use an icon without a visible or accessible text label for semantic state.

## 7. Core components

### `LensSwitch`

Two options: Projects and Places. It preserves the current query, filters, and selected object. The control explains the distinction on first use: “Projects show what belongs together. Places show where it was observed.”

### `SourceRootRow`

Contains source kind, exact root, age, coverage, policy, and counts. Source kinds use literal icons plus text: Local folder, Git repository, Network share, Cloud-synced folder.

### `ProjectRow`

Contains project identity, authority state, source summary, attention count, latest evidence, and membership basis. Expansion lists roots; it does not dump the full artifact inventory.

### `ArtifactRow`

Contains file/folder icon, display name, exact locator, artifact kind, project memberships, state, and latest observation. The main click selects; dedicated links open source or evidence.

### `StateChip`

One component family for compact semantic labels, with required icon + text. Variants are semantic, not arbitrary colors: policy, unknown, error, availability, completeness, status, and severity.

### `ConfidenceMark`

Shows a word label in normal density and a compact patterned mark in dense tables. The raw value appears on focus/hover and in the evidence drawer. Accessible name includes both: “Strong inference, confidence 0.95.”

### `Path`

Displays Windows-native paths without rewriting separators. Rules:

- preserve `C:\` and `\\server\share` roots;
- truncate middle segments, never the root or final name;
- expose the complete path on focus and through **Copy exact path**;
- use monospace and directional isolation for mixed scripts;
- distinguish repository-relative locators from absolute filesystem paths.

### `ClaimRow`

Presents subject → relation → object, confidence, assertion basis, and **Why?**. Examples:

- `connector_123.pdf` **renamed from** `archive\conn-123-old.pdf` — Strong inference · 0.95
- `widget_bom (copy).xlsx` **may match one of 2 files** — Weak candidate · 0.30

For ambiguous relations, group candidates into one compare-set presentation rather than repeating a misleading binary claim.

### `EvidenceDrawer`

A 440–520 px right-side drawer that preserves list/tree context. It opens from any claim, status, or “Why?” link and is deep-linkable.

Drawer anatomy:

1. **Claim** — plain-language statement, type, and subject.
2. **Assessment** — observed/inferred label, exact confidence, assertion precedence, and explanation.
3. **Evidence** — cited observations in chronological order.
4. **Interpretation context** — identity policy version, rule, extractor and version, effective content policy.
5. **Technical details** — locators, native versions, digests when permitted, visibility, raw JSON.

Use inline expansion for one supporting detail, the drawer for a normal evidence chain, and a dedicated full-page evidence view only when comparing many observations, versions, or candidates. Two conceptual levels—claim and cited observations—fit in the drawer; a third level such as raw event history moves to the full page.

### `ObservationTimeline`

Shows immutable observations in time order. The observation currently reflected in the projection is marked explicitly. Corrections point to the earlier observation instead of replacing it.

### `ProposalPanel`

Any future action is framed as a proposal with affected artifacts, evidence, predicted result, reversibility, and required approval. Primary action is **Review proposal**, never **Apply** from a summary row.

## 8. Evidence drill-down behavior

Example flow for an inferred rename:

1. The row says `Strong inference · 0.95` and offers **Why?**.
2. The drawer opens with: “Gyst considers this a rename because exactly one disappeared file and one newly observed file shared a content digest in the same completed scan pass.”
3. The relation metadata shows `renamed-from`, assertion basis, time, and policy version.
4. Two observation cards show old-location absence and new-location arrival, with observation IDs and timestamps.
5. Technical details expose connector/extractor versions and digest if policy permits.

Example flow for ambiguity:

1. The row says `Multiple candidates` and lists two candidates.
2. Each candidate is `Weak candidate · 0.30`.
3. The explanation says identical content prevents a unique pairing; it does not imply that one was rejected.
4. The available action is **Compare candidates**, not **Accept best match**.

Example flow for policy withholding:

1. The digest field says `Content not read` with a blue policy icon.
2. The drawer explains the effective content level and policy version.
3. It does not suggest rescanning as if the state were an error.

## 9. Density and scale

The interface should never render one hundred thousand file rows as the starting view.

### Default aggregation

- Start with projects or source roots.
- Collapse folders that contain no findings, ambiguity, or cross-project membership.
- Summarize homogeneous siblings: “2,418 generated files” with an expand action.
- Show counts with explicit scope: “12 open findings in 4,820 observed artifacts.”
- Keep findings, changed items, and unresolved candidates visible above routine inventory.

### Search and filtering

- Search is incremental and supports exact quoted paths.
- Filters are additive and always visible as removable chips.
- Every result states whether it matched name, path, project, identifier, or extracted metadata.
- Provide saved local filters without accounts or cloud persistence.
- Include “Only items needing attention” and “Show routine inventory” as clear modes.

### Rendering strategy for the static report

- Generate semantic summary HTML for immediate load and accessibility.
- Store detailed rows in locally bundled data inside the report; no network fetches.
- Use local JavaScript for indexed filtering, windowed rendering, and expansion.
- Keep browser history/deep links for selected project, source, artifact, and evidence drawer.
- Provide a no-JavaScript summary containing report metadata, projects, sources, finding counts, and export instructions.
- Benchmark at 10k, 100k, and 500k artifacts on representative Windows hardware. Initial targets: usable summary under 2 seconds, filter feedback under 100 ms, no scroll stalls above 50 ms.

The performance targets are prototype gates, not public guarantees.

## 10. Accessibility and Windows conventions

- Meet WCAG 2.2 AA for contrast, keyboard access, focus, names, and state.
- Use semantic tables for project lists and ARIA tree semantics only for the physical hierarchy.
- Support arrow-key tree navigation, Enter to expand/open, Escape to close the drawer, and a visible shortcut reference.
- Never require hover, drag, color perception, or right-click.
- Test at Windows display scaling of 100%, 125%, 150%, and 200%.
- Preserve drive letters, UNC paths, locked/unavailable-file states, and case-insensitive path expectations.
- Use locale-aware dates in summaries and an exact RFC 3339 timestamp in evidence details.
- Print/export views must preserve state labels, full evidence identifiers, and locally available license attributions.
- Respect reduced motion; transitions should clarify spatial continuity and complete in 120–180 ms.

## 11. Content design

### Voice

Calm, literal, and specific. Explain what was observed, what was inferred, and what Gyst could not know. Avoid celebratory dashboard language.

### Preferred patterns

- “Observed 8 minutes ago.”
- “Content not read under the source’s metadata policy.”
- “Two files are plausible matches. The evidence cannot distinguish them.”
- “The scan was interrupted after 84% of the source was visited.”
- “No current authority is declared.”
- “This relation was inferred from 2 observations.”

### Avoid

- “Gyst knows…”
- “Best match” when candidates are ambiguous.
- “Healthy,” “at risk,” or a traffic-light project score.
- “Sync” for observation or transfer-bundle import.
- “Deleted” when the evidence proves only absence.
- “Fix now” for a read-only proposal.
- “No data” when the real state is withheld, restricted, unavailable, or not scanned.

## 12. MVP screens and states

### Screen A: Discover — Projects

Proves cross-source membership, authority language, attention summaries, and source freshness.

### Screen B: Discover — Places

Proves Windows roots, source-kind differences, physical hierarchy, multiple project memberships, unassigned artifacts, and partial scans.

### Screen C: Evidence drawer

Proves the full claim → reason → observation chain for observed facts, high-confidence rename, ambiguous compare-set, and policy-withheld content.

### Screen D: Compare candidates

Proves that ambiguity can be actionable without pretending to know. Shows two to four candidates, comparable metadata, exact reasons, and a future-safe proposal path.

### Required fixture scenarios

1. One project spans two repositories and one network share.
2. One folder belongs to two projects.
3. One artifact has no project membership.
4. A one-to-one rename appears at 0.95.
5. An ambiguous duplicate produces two 0.30 compare-set candidates.
6. An empty file is explicitly skipped for content matching.
7. Content is withheld by metadata policy.
8. A network scan is three days old and interrupted.
9. A cloud placeholder is present but content is unavailable locally.
10. Evidence is redacted because the viewer lacks visibility.
11. A finding is waived with actor, reason, time, and expiration.
12. A source contains at least 100,000 artifacts.

## 13. Design and implementation plan

### Phase 0 — Contract inventory and sample corpus (1–2 days)

Deliverables:

- representative generated JSON for observations, relations, findings, identity groupings, discovery roots, pass status, and projects;
- a state matrix covering unknown, withheld, error, unavailable, not observed, partial, and redacted;
- confirmation of fields that are facts versus projections;
- a report-size baseline from the synthetic dataset plus a generated 100k-artifact dataset.

Exit criteria:

- every visible claim can be mapped to a field or explicitly marked as a proposed derived presentation;
- source freshness and pass-completeness fields are defined;
- no mock data contradicts the schemas or CLI behavior.

### Phase 1 — Interaction model (2–3 days)

Create low-fidelity, data-filled prototypes for:

- Projects lens;
- Places lens;
- synchronized selection when switching lenses;
- evidence drawer;
- ambiguous candidate comparison.

Test with five tasks:

1. Find the current candidate for a named artifact.
2. Identify every place contributing to a project.
3. Explain why Gyst inferred a rename.
4. Explain why Gyst refused to choose between two files.
5. Distinguish policy withholding from a failed scan.

Exit criteria:

- users can explain the difference between a project and a folder;
- users do not interpret 0.30 as 30% complete or 30% severe;
- users can reach supporting observations in two interactions or fewer;
- the tree is understood as a physical-location view.

### Phase 2 — Visual system and components (2–3 days)

Build tokens and component states in plain HTML/CSS using only bundled assets and system fonts. Cover default, hover, focus, selected, disabled, loading, empty, error, policy-limited, and restricted states.

Prioritize:

1. typography, spacing, focus, and layout;
2. tables, trees, paths, and filters;
3. confidence and semantic state marks;
4. evidence drawer and observation timeline;
5. empty and large-result states.

Exit criteria:

- all semantic states work without color;
- all components render correctly at Windows scaling from 100% to 200%;
- website brand tokens are recognizable without compromising application density.

### Phase 3 — Real static-report prototype (3–5 days)

Integrate the components into the generated report, driven by real schemas and CLI output. Add local search, filters, deep links, windowed lists, and accessible keyboard interactions. No remote fonts, icons, scripts, analytics, or APIs.

Exit criteria:

- report works offline when opened on Windows;
- no visible fact lacks an evidence path or clear state explanation;
- Places and Projects preserve selection and filters;
- the 100k-artifact fixture remains responsive;
- print/export provides a durable human-readable summary.

### Phase 4 — Validation and hardening (2–3 days)

Run:

- task-based usability sessions with at least three engineers or engineering managers;
- keyboard-only and screen-reader review;
- Windows path, UNC share, placeholder, unavailable source, and locked-file tests;
- narrow-window, long-name, localization, and 200% scaling tests;
- review against the human boundary and read-only guarantee.

Track misunderstandings, not preferences. A visual treatment fails if a user reads an inference as a fact, a policy choice as a failure, or a path as project identity.

### Phase 5 — Handoff (1 day)

Deliver:

- versioned token sheet;
- component/state inventory;
- annotated HTML prototype;
- interaction notes and keyboard map;
- schema-to-component mapping;
- accessibility and scale test results;
- unresolved decisions with evidence from testing;
- locally bundled icon/license manifest updates.

## 14. Acceptance criteria for the first view

The first view is ready to ship when:

- a user can see a project spanning multiple physical roots without mistaking one root for the project;
- a folder can visibly belong to multiple projects;
- observed facts and inferences are visually and verbally distinct;
- 0.95 and 0.30 relations do not look equivalent;
- unknown, policy-withheld, error, unavailable, and redacted states are distinguishable without color;
- freshness and scan completeness appear separately;
- every derived claim opens the supporting evidence and interpretation context;
- no primary workflow requires network access;
- no action suggests immediate mutation of source files;
- 100,000 artifacts remain navigable through aggregation, search, filters, and windowed rendering;
- Windows paths and UNC roots remain legible and copyable;
- no project, team, or person receives an opaque health/productivity score.

## 15. Open decisions to resolve with the prototype

These should remain reversible until tested:

- the confidence band thresholds and exact labels;
- whether the first post-discovery default should be Projects or the user’s last lens;
- source-specific freshness cadence defaults;
- whether membership chips show only the first two projects before “+N”;
- whether the evidence drawer defaults to a plain-language or technical tab;
- whether a large static report is a single HTML file or an offline report folder while preserving a no-server experience;
- the smallest useful project-detail view beyond the overview expansion;
- how user-confirmed ambiguity resolution becomes a durable assertion in a later interactive product.

## 16. Immediate next artifact

Build one high-fidelity, static, data-backed Discover prototype with Projects and Places lenses plus the evidence drawer. Populate it with the twelve required fixture scenarios above. This single artifact will test the hardest product claims—cross-folder projects, honest uncertainty, policy visibility, freshness, evidence, and density—before expanding the system into release or manufacturing views.
