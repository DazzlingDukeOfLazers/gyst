# Handoff log

Running log between the design side and the code side. Newest entry first.
Entries are short: what is needed or done, where in the repo, and what is
blocked. Answer in place under the same heading. See `AGENTS.md` for the
conventions.

## 2026-09-21 — Claude: the review loop exists

Built as you specified, command line first, report read-only:

1. `gyst review` shows the queue with reviewed and remaining counts,
   ordered by consequence: nested boundaries, cross-project findings, open
   findings, generic names, size. On the real tree: 30 candidates, 0
   reviewed.
2. `gyst review <candidate>` explains one and previews Confirm and Ignore
   in plain language, including what will not change.
3. `gyst assert project confirm|ignore <id> --by --reason` records the
   assertion with actor, reason, time, and the record's evidence, refuses
   declared records and double judgments, rebuilds the projection, and
   prints the new counts and "nothing renamed, moved, or deleted".
4. `gyst assert retract <id>` restores the lower-precedence evidence; the
   retraction stays in the history.
5. The report carries `projects[].state`, `projects_*` counts, and
   assertions with `subject_kind`. The page leads with your summary,
   opens on Needs review, shows state in words, and in the drawer shows
   the commands and an Audit list. `docs/day-23-notes.md`.

Proved on the fixture with a tree-digest check that nothing on disk
moved. Alias, containment, same-project, include, and exclude are not
built; they reuse this loop once the vocabulary is agreed.

## 2026-09-21 — Chad: make candidate review actionable before broad UI work

Daniel asked what a user is supposed to do with the current Projects report.
The honest answer is that it supports inspection but does not yet let a person
finish a task. I updated `docs/project-inventory-curation.md` sections 1, 4, 14,
and 15 to define the first user job and change the implementation order.

Please make the next outcome after basic report legibility one thin vertical
slice:

1. Show the Needs review queue and remaining count.
2. Explain one candidate and preview the consequence of **Confirm as project**
   or **Ignore as project**.
3. Record the chosen assertion with actor, reason, time, and evidence through a
   trusted write boundary. CLI first is acceptable; the exported HTML must stay
   read-only.
4. Regenerate or refresh the projection, show the changed state and count, and
   expose the decision in Audit.
5. Retract it and demonstrate that lower-precedence evidence is restored and no
   scanned source file changed.

Do not block this slice on alias, containment, same-project, member overrides,
or the eventual interactive application. Those can reuse the loop afterward.
The second user job—finding the appropriate artifact for a particular edit,
release, manufacturing, documentation, or handoff task—remains intentionally
incomplete until concern-scoped authority has been researched. Artifact-level
authority counts are diagnostic and must not be presented as that answer.
## 2026-09-21 — Claude: 10.2 and 10.3 landed

`projects[].boundary {source_id, locator}` and `projects[].physically_within`;
`findings[].projects[]` and `findings[].cross_project`. The page uses both:
generic names show their parent, findings group by membership with
cross-project combinations first. `docs/day-22-notes.md`. Sample regenerated.

## 2026-09-21 — Claude: contract answers for the curation design (84a375f)

Accepted as the direction. The critique of the first page is right on
every point; it was a contract reader, and the first-path finding grouping
was a placeholder. Answers to section 10, then the order I propose.

**10.2 Boundary locators.** Easy, and I will add it first. Every project
record gains `boundary: {source_id, locator}`: the folder that carries the
marker, or the folder holding the manifest. Alongside it,
`physically_within: <project_id> | null`, the nearest other record in the
same source whose boundary is a path prefix, computed from the two locators
and nothing else. The field name says what it is: physical nesting, not
containment. Containment is an assertion (10.1) and will be a separate field.

**10.3 Finding-to-project.** Also now. Every finding gains
`projects: [project_id, ...]`, the union of memberships of its *file*
subjects, and `cross_project: true` when they span more than one. Subjects
that are not files (a source root in a stale-source finding) contribute
nothing and the list may be empty. Nothing new can leak: a finding's
subjects already name the files, and the report carries no viewer
filtering. When export-time visibility filtering exists, it applies to
findings and their subjects together. Please stop deriving this on the
client once it lands; the projection is the one place it should be computed.

**10.4 Observation contents.** The report will embed a `cited_observations`
section: every observation whose id is cited by any relation, finding,
assertion, or authority record, as a compact record (id, source, locator,
kind, claim type, observed_at, connector and version, digest, size, content
level, extractor name and version, warnings). Not the full payload; the
manifest and marker payloads are already reflected in projects. On the real
tree that is roughly 30,000 records, tens of megabytes; on the fixture,
dozens. The drawer can then show what a cited observation said. The
`visibility_scope` sentence will state that cited observations are included.

**10.1 Project-curation assertions.** The largest, and I want the shape
agreed here before code. Extend the existing `assertions` table rather than
add a second: it already has actor, reason, evidence, time, retraction with
who and why, and a trigger that forbids edit and delete. Today its subject is
a file. It gains `subject_kind` (`file` | `project`), `object` (a second
project id, or a `source_id:pattern` member), and `value` (an alias or
description), and its `kind` enum grows:

| kind | subject | object / value | effect in the projection |
|---|---|---|---|
| `project.confirm` | candidate id | — | state becomes `confirmed`; basis stays native-marker; confidence 1.0 |
| `project.ignore` | candidate id | — | state becomes `ignored`; memberships withdrawn; record kept, listed under Ignored |
| `project.alias` | project id | value: name, description | `name` shows the alias; `declared_name` keeps the original |
| `project.contains` | parent id | object: child id | child gains `parent_project_id`; nothing about membership changes |
| `project.same` | project id | object: project id | the two records merge into one with `also_known_as`; memberships union; the lexically first id is canonical |
| `member.include` | project id | object: `source:pattern` | an explicit member at confidence 1.0, basis `explicit` |
| `member.exclude` | project id | object: `source:pattern` | files matching are removed from that project's membership |
| `retract` | (existing) | — | as today: recorded on the row, effect withdrawn |

Precedence follows design-round-2 section 2: explicit beats manifest beats
marker. Retracting an assertion restores whatever the lower-precedence
evidence says. Candidate ids (`mark_…`) are stable across scans for the same
source and folder, so they can be subjects; if a folder moves, the assertion
dangles and the report will list it under a `stale_assertions` count rather
than apply it to something else.

Report additions from this: `projects[].state` (`declared` | `confirmed` |
`candidate` | `ignored`), `projects[].declared_name`, `parent_project_id`,
`also_known_as`, `excludes[]`, and `assertions[].subject_kind / object /
value`. No removals.

**Order.** One PR now with 10.2 and 10.3 (contract additions only, no
schema change). A second with 10.4. A third with 10.1 once you say the
table above is the vocabulary you want: it is a schema migration on both
engines, a projection change, and `gyst assert project …` commands. Stage
A on your side needs only the first; the overview and the Audit tab need
the second and third.

**Section 13.** The privacy review is the most important page in the
document and the report has none of it yet. Path redaction and
audience-specific presentation need the export-time visibility filter that
ADR 004 left open; I will bring it forward as its own record rather than
bolt it onto 10.4.

## 2026-09-21 — Chad: project inventory and curation design

The first real report review is in
`docs/project-inventory-curation.md`. It turns the flat Projects table and
project file dump into a candidate-review flow, a summarized project workspace,
membership-based finding groups, and artifact-level authority views. Most Stage
A work derives from the current report contract.

Four code-side contract questions are called out in section 10: durable
append-only project-curation assertions, explicit physical boundary locators,
finding-to-project projection semantics, and observation content for the
evidence view. Scoped project authority is deliberately deferred; do not add a
single project “source of truth” field. Please answer here with the contract
shape and implementation order before the client invents fields.
## 2026-09-20 — Claude: you can send me data as a signed file now

`gyst key init <you>`, scan with `--egress facility`, `gyst export`, and
send the `.jsonl`. I import it with `--trust-on-first-use`, your sources
show up as `<you>/…` in my store, and the report includes them. If you
scan a folder that also holds a copy of something in mine, the duplicate
finding spans both of us, which is the cross-organisation case the
fixture cannot show. ADR 006 has the format; it is one line of header and
one observation per line, readable in a text editor. This is also the
"deliver files as messages" answer to the last question in the brief.

## 2026-09-20 — Claude: there is a page now

`gyst report --html report.html` writes one static file with the two
lenses, grouped findings, and an evidence drawer over the JSON contract.
It is deliberately plain: your tokens and type, none of your components.
Run it on the fixture or on a real tree and replace it; the JSON in the
page's script element is the same document `--out` writes. What it
taught: at 81,000 files one file is 163 MB and still loads in under ten
seconds, and folding vendored subtrees by default is what makes the Places
lens readable. `docs/day-19-notes.md`.

## 2026-09-20 — Claude: first real scan, and three contract changes

Scanned 23 real repositories, 81,000 files. `docs/day-18-notes.md` has
the numbers. Three things in the report contract changed as a result:

1. `files[].vendored` is new: true for files under dependency, build,
   cache, and tool directories. They are observed but are not project
   markers, duplicate findings, or authority candidates. The Places lens
   should probably fold them by default; 83% of a real tree is them.
2. `artifacts[]` now lists only groupings with more than one member. A
   single-file grouping is already in `files[].grouping`.
3. Manifest `members` patterns are relative to the manifest's own folder,
   not the scan root. The fixture's manifests now say `**`. The resolved,
   root-relative pattern is what `projects[].members[].pattern` carries,
   so your side sees no change in shape.

Also: a real tree produces 1,412 open findings after the noise is
removed. The findings view needs grouping by project and by rule before
it is readable; the report has the fields for both.

## 2026-09-20 — Claude: no server needed to run Gyst any more

`GYST_DATABASE_URL=sqlite:gyst.db` runs everything on a file: scan, git,
identity, assert, findings, report. The report is byte-identical to the
PostgreSQL one apart from clocks. If you want to generate your own
fixture data on your machine without installing anything but Go, this is
how; `docs/samples/README.md` has the recipe. The lists in the report are
now sorted by byte order rather than by database collation, so the same
input gives the same document on every machine.

## 2026-09-20 — Claude: confidences in the report are exact now

Small but visible: every `confidence` in the report is now a clean
decimal (0.6, 0.88), where a few were float32 artefacts like
0.6000000238. Migration 0010 stores them as double precision. No shape
change. This fell out of drawing the store boundary (ADR 004 stage 1).

## 2026-09-20 — Claude: authority is its own claim now

Done, in `migrations/0009_assertions_authority.sql`, `internal/authority`,
`gyst assert`, and `docs/day-12-notes.md`. Your correction shaped it:
membership and grouping are not consulted as authority.

- `files[].authority` in the report is `{state, basis, authority, confidence,
  evidence, assertion_id, explanation}` with state `declared`, `likely`,
  `multiple`, or `none`. These map one-to-one to your Declared authority,
  Likely authority, Multiple authority candidates, and No authority
  identified.
- `assertions[]` lists every person's statement, active and retracted,
  with actor, reason, time, and cited observation. Retractions are kept.
- `report.counts` has the four state counts. In the sample, one file is
  declared, its byte-identical copy follows it, and everything else is
  none or multiple.

Not yet: any authorization of who may assert; that is deployment policy.

## 2026-09-20 — Claude: replies to the design responses (cd66f39)

Taking your five contract priorities in order:

1. **Versioned JSON export referencing evidence ids.** Done: `gyst report`,
   see the entry below and `docs/samples/fixture-report.json`. Metadata has
   generation time, generator version, schema id, active identity policy,
   and a `visibility_scope` sentence. Every derived value cites observation
   ids; explanations are carried alongside, not instead.
2. **Authority as a separate claim.** Agreed, and nothing in the code or the
   report says otherwise. The report has no authority field. Note that
   `files[].grouping.is_current` is the identity profile's current member of
   a group, not authority; please do not render it as authority.
   `docs/day-11-notes.md` restates this.
3. **Hand-authored expected membership with one file in two projects.** Done:
   a second manifest at `engineering/.gyst/project.yaml`; every widget file
   is now in `engineering` and `widget` (10 of 20). Expectations live on each
   entry in `testdata/generate.py` and in `expected-inventory.json` under
   `files[].projects`, `projects`, and `suppressed_markers`. A test resolves
   membership from the tree and checks every file against them.
4. **Findings output.** Done: six rules, `gyst findings --json` validates
   against the schema. The sample report has one real finding.
5. **Expected cadence.** Done with findings: `--cadence` per source, defaults
   by location kind. The report derives `current`, `due-soon`, `stale`,
   `interrupted`, `unavailable`, `never-scanned` at generation time and
   carries coverage separately, so recent+interrupted and old+complete are
   both expressible. If you prefer to keep freshness labels deferred until
   you have tested them, the inputs are all there to compute your own.

Your placeholder state "Content unavailable locally" matches how it is
recorded: `files[].placeholder` is true, `content_level` is whatever policy
said, and the observation's warning explains it. Distinct from withheld,
error, and unknown, as you specified.

## 2026-09-20 — Claude: the report document is the contract

Done, in `internal/report`, `gyst report`, and `docs/samples/fixture-report.json`
with `docs/samples/README.md`. This is the "representative generated JSON"
Phase 0 asks for: sources with freshness state, projects with members, every
file with memberships and grouping, artifacts, relations, and findings, in
one document, each derived value next to the observation ids behind it.

The freshness state (`current`, `due-soon`, `stale`, `interrupted`,
`unavailable`, `never-scanned`) is computed from age, coverage, and cadence
at generation time, exactly as section 5 describes. Coverage is carried
separately so "seen 8 min ago + interrupted" is expressible.

Not yet in the document: the cited observations themselves (only their ids),
and any export-time visibility filtering. `visibility_scope` says so.

Questions for you: does `files[].grouping` plus `artifacts[]` give you what
the evidence drawer needs, or do you want the observation records inline?
And is one JSON document the right unit, or should large sections be
separate files in a report folder?

## 2026-09-20 — Claude: findings

Done, in `migrations/0008_findings.sql`, `internal/findings`, and
`docs/day-9-notes.md`.

- Six rules: duplicate content, ambiguous origin, invalid manifest, and
  stale, unavailable, or interrupted source. Severity, status, subjects,
  evidence, confidence, summary, remediation, and waiver exactly as
  `schemas/v0/finding.schema.json`. `gyst findings --json` emits it and
  validates. This is the "Attention" column and the Findings view.
- Status is `open`, `acknowledged`, `waived`, or `resolved`. Dispositions
  survive rescans. A waiver records actor, reason, time, and optional
  expiry; only a user may waive. Fixture scenario 11 is now producible.
- Cadence per source (`--cadence 7d`), defaulting to 7 days for a share
  and 1 day otherwise, so "stale" is relative to expectation. This is the
  freshness state machine in design-system.md section 5, minus "due soon",
  which is a presentation threshold the report can compute from cadence.

Open for you: should "N files present as cloud placeholders" be a finding
or a property shown on the source? I left it out of findings for now.

## 2026-09-20 — Claude: projects and membership

Done, in `migrations/0007_projects.sql`, `internal/manifest`,
`internal/project/membership.go`, and `docs/day-8-notes.md`.

- `projects`, `project_members`, and `file_projects` tables, rebuilt from
  the log on every scan. Basis is `manifest` (confidence 1.0) or
  `native-marker` (0.6); `explicit`, `organization-rule`, and `suggestion`
  are defined but nothing produces them yet.
- A file can be in several projects; `gyst explain <file>` lists them with
  basis, confidence, and the pattern that matched. This is the membership
  chip and the "Membership" column in design-system.md section 3.
- One manifest id in two sources is one project. This is fixture scenario 1.
- `gyst projects` lists projects with file and source counts.
- New claim type `project.manifest` in `schemas/v0/observation.schema.json`,
  with `schemas/examples/valid/observation.project-manifest.json`.

~~Authority state is still not produced.~~ Done, see the authority entry.

**Chad response:** Accepted for the Projects and Places prototype. The project
and membership records are enough for project/source counts, membership chips,
and the Membership column.

One semantic correction: a project manifest currently declares the project and
its membership; it does not declare which member artifact or version is the
authority. The Authority column must therefore remain "No authority identified"
until authority has its own assertion or is derived from appropriate identity
evidence. Do not turn manifest membership confidence into artifact-authority
confidence.

Before report integration, the fixture still needs hand-authored expected
projects and one explicit case where the same file belongs to two projects.

## 2026-09-20 — Claude: source location, placeholders, and discovery

Done, in `migrations/0006_source_location.sql`, `internal/location`,
`internal/discover`, and `docs/day-7-notes.md`.

- Every source now records a location: kind (`local`, `network-share`,
  `cloud-synced`, `unknown`), provider (`apfs`, `smb`, `dropbox`, ...), the
  mount or sync root, an evidence sentence, and a confidence. This is the
  source-kind icon and label in design-system.md sections 3 and 7.
- Cloud placeholder files are observed by metadata only and never opened.
  The observation's claim payload carries `"placeholder": true` and a
  warning. This is fixture scenario 9.
- `gyst discover --json` lists candidate project roots with markers and
  location. This is the "discovery roots" JSON from Phase 0.

Not verified against a live share, UNC path, or real OneDrive placeholder;
unit tests only. If you have a Windows machine with OneDrive, running
`gyst scan` on a Files On-Demand folder and sending the output would confirm
scenario 9 end to end.

**Chad response:** Accepted as the source-location contract for the first
prototype. The UI will map the stored kinds to Local folder, Network share,
Cloud-synced folder, and Unknown, while keeping provider and classification
evidence in the evidence drawer.

A placeholder will be a distinct state: "Content unavailable locally." It is
not policy-withheld, a scan error, or unknown. Live UNC and OneDrive verification
remains an explicit test gap; the design will not present the behavior as
Windows-verified until that test exists.

## 2026-09-20 — Claude: scan passes are recorded

Done, in `migrations/0005_scan_passes.sql` and `docs/day-6-notes.md`. Every
scan now records a pass with `started_at`, `finished_at`, and a coverage
status of `complete`, `partial`, `interrupted`, or `unavailable`, plus a
detail string saying why. `gyst status` shows the latest pass per source and
prints `never scanned` for a registered source with no pass.

This gives the design its "age" and "coverage" values for a source root
(design-system.md section 5, freshness). Expected cadence, and therefore the
current / due soon / stale distinction, is still not recorded.

**Chad response:** Accepted. Until expected cadence is part of the contract, the
prototype will show elapsed age plus the exact timestamp and the independent
coverage value. It will not label a source Current, Due soon, or Stale from age
alone. Recent + interrupted and old + complete must remain valid combinations.

## 2026-09-20 — Claude: fields the design needs that code does not emit yet

From reviewing `docs/design-system.md` against the current schemas and
migrations. None of these block design work on fixtures, but the prototype
should not invent shapes for them; the shapes will arrive through
`schemas/v0/` and `schemas/examples/`.

- ~~Source kind beyond `local-folder` and `git`~~ done, see entry above.
- ~~Persisted scan pass with status~~ done, see entry above.
- Expected scan cadence per source.
- ~~Projects, membership, and membership basis~~ done, see entry above.
- Authority state: declared, likely, multiple candidates, none identified.
- ~~Findings with severity, status, and waiver~~ done, see entry above.
- ~~Cloud placeholder state~~ done, see entry above.
- Export visibility scope on the report itself, since a static file has no
  viewer to filter for.

Planned order on the code side: scan passes and source classification, then
project manifests and membership, then findings, then JSON export.

**Chad response:** The first data-backed report can now begin against source,
pass, discovery, project, membership, observation, and relation fixtures. The
remaining contract priorities from the design side are:

1. A versioned JSON report export that references evidence IDs rather than
   flattening explanations into display-only strings. Its report metadata needs
   generation time, Gyst/generator version, schema versions, and export
   visibility scope.
2. Authority as a separate artifact/version claim. Project membership, including
   manifest membership at confidence 1.0, must not imply authority.
3. Hand-authored expected project membership in `testdata/expected-inventory.json`,
   including one artifact in two projects.
4. Findings output when the first real rule emits it. The design can exercise the
   existing schema example, but will not invent report finding counts.
5. Expected cadence when scheduling policy exists. It does not block the first
   prototype; freshness labels stay deferred.

Export visibility scope blocks distribution of a portable report, because a
static file has no viewer-time ACL check. It does not block a clearly labelled,
local-only prototype generated for the scanning principal.
