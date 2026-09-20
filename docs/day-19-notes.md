# Day 19 notes: a first page

`gyst report --html report.html` writes one static page. The report JSON
sits in a script element, and a small inline renderer reads it: the
Projects lens and the Places lens the design describes, findings grouped
by rule and by project, and an evidence drawer. No external assets, no
network, no server. It uses the design system's tokens and type and
nothing else of the design system, because it is a first reading of the
contract for the designer to replace, not a prototype of the design.

## What it shows

**Projects** is a table: name, basis, confidence, files, sources, member
patterns. Clicking a row opens the drawer with the claim, the members,
the evidence ids, and the files.

**Places** is one tree per source, with the source's location, freshness
state, and coverage in its header. Every file carries its membership
chips, so a file in two projects shows two, and a chip for vendored,
content-not-read, placeholder, and open findings. Vendored subtrees are
folded by default; a checkbox unfolds them. 83 percent of a real tree is
vendored, and a tree that shows them by default shows nothing else.

**Findings** groups open findings by rule and by the folder of their
first subject, largest group first, each expandable. The 1,412 findings
of the real tree become forty groups, which a person can read.

**The drawer** shows, for a file: what was observed, with digest, native
version, time, and observation id; the authority state with its reason
and the cited evidence; memberships with basis, confidence, and the
pattern that matched; the grouping under the active profile; and open
findings that name the file. Escape closes it.

## Two rules kept from the design

"Observed" belongs to observations. A manifest membership says
*declared*, a marker says *suggestion*, a profile rule is named; the
number sits beside the word either way. And severity is not confidence:
findings show severity as a word and colour, and never a number.

## Size

The page for the fixture is 40 KB. The page for the real tree, 81,000
files, is 163 MB and loads in under ten seconds in the desktop browser,
with the JSON parsed once and indexed in memory. That answers the
design's open question for now: one file works at this scale. It would
not at ten times the scale, and the vendored files, which are 83 percent
of the document and folded on sight, are the obvious thing to leave out
of the page or load on demand.

## What this does not do

- No search across the drawer's contents, no keyboard navigation of the
  tree, no deep links, no print view. The design's Phase 3 list.
- Observations are cited by id, not shown. The drawer can say which
  observation supports a claim but not what it said; the report does not
  embed observations yet.
- Windows paths are not tested; locators are forward-slash by contract
  and roots are shown as the source recorded them.
- It is one HTML string in a Go file with the styling inline. The
  designer's tokens and components should replace it, and the JSON
  contract is what they build against.
