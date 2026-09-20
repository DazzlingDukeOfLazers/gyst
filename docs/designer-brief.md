# Designer brief: the first Gyst views

This is context, a request, and a set of questions for the designer joining the
Gyst project. It is written to stand alone. The longer design record lives in
the other files in this folder; the ones worth reading first are linked at the
end.

The designer's response, a proposed design system and first-view plan, is in
[design-system.md](design-system.md). It answers most of the questions below
and defines the fixture scenarios and acceptance criteria the code must meet.

## What Gyst is

Gyst is a free, open-source tool that helps engineering teams find their real
work products, understand what changed, and prove what went into a release. It
meets people where their work already is: Git repositories, local folders,
network shares, cloud-synced drives, spreadsheets. It does not ask anyone to
move their files into a new system first.

The one-sentence promise:

> Find the authoritative engineering files. Explain what changed. Prove what
> went into the release. No migration required.

The product is organized around four questions. Treat them as the information
architecture:

1. **What is this?** An artifact, its project context, owner, and relationships.
2. **Where is the authority?** The canonical file or record, and which other
   things are copies, exports, caches, or generated outputs.
3. **What changed?** Human-meaningful revisions, not just byte-level diffs.
4. **What needs attention?** Stale data, broken references, duplicates,
   ambiguity, unreproducible outputs.

## Where the project is

Gyst is pre-alpha. The command-line tool can scan folders, record what it saw in
an append-only log, observe Git history, group files under reversible naming
rules, explain the evidence behind any result, notice deletions, and
conservatively detect renames. There is no graphical interface yet.

This round adds two things the design work depends on:

- **Discovery.** Finding project roots across a disk and across mounted network
  shares and cloud-synced folders, and recording what kind of place each root
  lives in.
- **A first view.** Something that shows those projects. A tree is the obvious
  starting point. We think it is the right starting point and the wrong
  destination, for reasons below.

The first user is an engineer or engineering manager asking "where is the
current version of this thing?" Executive rollups and manufacturing views come
later and are not part of this round.

## Vocabulary you will see in the data

| Term | Meaning |
|---|---|
| Source | One place Gyst looks: a folder, a share, a repository. Has a root and a kind. |
| Artifact | A file, folder, commit, or other work product Gyst knows about. |
| Observation | One immutable record of what a source looked like at a moment. Everything else is derived from these. |
| Relation | A typed link between artifacts. Current types include `renamed-from`, `compare-set-with`, `duplicate-of`, and `generated-from`. |
| Finding | A problem or question worth a human's attention, with its evidence and a status. |
| Tombstone | An observation that an artifact is now absent from where it was. |
| Project | Any useful collection of artifacts. Many-to-many with folders and repositories. |
| Confidence | A number from 0 to 1 attached to every derived claim. |
| Content level | Per-source policy: whether Gyst read file contents, only metadata, or nothing. |

## Facts about the product that shape the design

**Projects are not folders.** One project can span three repositories and a
share. One folder can belong to two projects. A filesystem tree shows where
bytes physically are, which is useful evidence, but a tree alone will tell the
user something false about what Gyst exists to explain.

**Unknown and ambiguous are legitimate answers.** Gyst deliberately refuses to
guess. When two files could each be the moved version of a deleted one, it
reports "one of these" at low confidence rather than picking. Here is real
output from the current tool:

```
renamed-from      0.95  archive/conn-123-old.pdf  <- connector_123.pdf
compare-set-with  0.30  archive/bom-a.xlsx        <- widget_bom (copy).xlsx
compare-set-with  0.30  archive/bom-b.xlsx        <- widget_bom (copy).xlsx
empty.txt                                          skipped, no content to match on
```

The interface must show that honestly without looking broken. We think this is
the hardest design problem in the product.

**Every fact links to evidence.** Any claim on screen must be traceable to the
observation, the extractor version, and the time it was seen. "Why do you think
this?" is a first-class interaction, not a debug feature.

**Confidence and policy are visible, not hidden.** A relation at 0.95 and one at
0.30 are different objects. "Content withheld by policy" is a normal state and
must read differently from "unknown" and from "error".

**Freshness matters.** A network share last scanned three days ago, with the
scan cut short, must read differently from a local folder scanned a minute ago.
Sources have kinds with different behaviour:

| Kind | What it means for the user |
|---|---|
| Local folder | Fast, complete, current. |
| Git repository | Has history and authorship; a commit is not automatically a release. |
| Network share | Scanned on a schedule, may be slow or unavailable, may be incomplete. |
| Cloud-synced folder | Files may be placeholders not yet downloaded; Gyst avoids touching contents. |

**Hard constraints.** These are product guarantees, not preferences:

- Runs fully offline. No remote fonts, icons, scripts, analytics, or accounts.
- Windows is the first-class platform. Design for drive letters, backslashes,
  and UNC share paths, not macOS conventions.
- Read-only by default. Any action the interface offers is a preview of a
  proposal, never an immediate change to the user's files.
- No people rankings, activity scores, or health scores. Gyst observes work
  products, not workers. Ownership may route a question; it may not rate a
  person.
- A vague single "project health" number is explicitly out of scope until its
  inputs and uncertainty are visible next to it.

## What already exists

- A one-page project website in `site/`, with a considered icon set and the
  reasoning behind each icon in `site/icon-search-map.md`.
- A synthetic messy dataset in `testdata/` with an expected inventory, built to
  contain the awkward cases: copies, renames, empty files, generated outputs.
- Real command-line output from `gyst explain`, `gyst changes`, and
  `gyst identity status`, which we can generate on demand.
- JSON schemas in `schemas/v0/` describing observations, artifact references,
  relations, and findings. These are the contract the interface will consume.

We would rather hand you real generated output than wireframes. The plan is for
the tool to emit a static HTML report with no server behind it. You can restyle
and restructure that report directly, and what you produce ships.

## What we are asking for

A starting point for the first view, not a finished design system. Concretely:

1. A way to show discovered projects and the sources they span, starting from a
   tree of where things physically are and moving toward the project view.
2. A visual language for confidence, for "withheld by policy", for "unknown",
   and for freshness, that works across both views.
3. An evidence drill-down pattern: how a user gets from a claim to the
   observations behind it.
4. A point of view on density, because real teams have a hundred thousand files.

## Questions for you

- How have you shown "I don't know" in a tool before? Did users trust the tool
  more or less because of it?
- Given that projects cross folders, what is your instinct for the first
  navigation model: a tree with overlays, a list with facets, a graph, something
  else? What would make you distrust a tree?
- What do you need from us to start: real data, a stable JSON contract, static
  HTML you can restyle, a component boundary?
- Where does evidence drill-down live? Inline expansion, a side panel, a
  separate screen? How many levels deep before it becomes its own page?
- What is your threshold for too much? How do you approach progressive
  disclosure at a hundred thousand files?
- How should confidence read visually: numbers, bands, words? How should
  "withheld by policy" differ from "unknown" and from "error"?
- Is the physical location of a file, meaning local disk, share, or cloud, a
  primary facet or a detail?
- Are system fonts and bundled icons acceptable, given no network at runtime?
- Can you work from Windows, or at least design against Windows path and window
  conventions?
- How do you want to receive feedback and deliver iterations? We are building a
  way for people to exchange signed files as messages between their own local
  copies of Gyst. If that would work for you, we would like to use it with you.

## Read next

- [Design system and first-view plan](design-system.md), the designer's
  response to this brief.
- [Product definition](product.md), especially "Product promises", "Core
  concepts", and "Human boundary".
- [Day 5 notes](day-5-notes.md), a short account of how and why the tool refuses
  to guess about renames.
- [Decisions](decisions.md), the register of what is settled and what is open.
- [The site](../site/README.md) and its [icon map](../site/icon-search-map.md).
