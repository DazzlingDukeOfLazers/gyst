# Day 10 notes: the report document

Everything the engine knows now comes out as one JSON document, and that
document is the contract between the engine and the static report the
design describes. `gyst report` writes it; `docs/samples/fixture-report.json`
is a real one from the fixture.

## What is in it

| Section | Contents |
|---|---|
| `report` | Schema, generator, generation time, visibility scope, active identity policy, counts. |
| `sources` | Each source with its location, cadence, latest pass, and a freshness state. |
| `projects` | Each project with basis, confidence, explanation, evidence, member patterns, file count, and the sources it spans. |
| `files` | Every locator in the current inventory: presence, size, digest, native version, observation id, content level, placeholder flag, project memberships, and its grouping under the active identity policy. |
| `artifacts` | Groupings under the active policy with their members. |
| `relations` | Every relation with type, endpoints, precedence, actor, evidence, confidence, explanation. |
| `findings` | Every finding, in the v0 finding schema, including resolved ones. |

Every derived value sits next to the observation ids that support it. That
is what lets the report's evidence drawer work without a server: the ids are
the join key back into the observation log, and a future version can embed
the cited observations themselves.

## Freshness is derived here, not stored

The design wants a source to show one of: current, due soon, stale,
interrupted, unavailable, never scanned. The database holds age and coverage
as two values, which is right; the state is a presentation over both and the
source's cadence, so it is computed at generation time by a pure function
with a test. Due soon is the final fifth of the interval. A partial pass that
is recent is still current: coverage is shown beside the state, not folded
into it.

## Visibility scope is a field

A static file has no viewer to filter for. Whoever holds it holds all of it.
Rather than pretend otherwise, the document says so in `visibility_scope`.
When export-time filtering exists, this field is where it will say what was
left out.

## What this does not do

- The observations themselves are not embedded, only cited. For the fixture
  that would be small; for a hundred thousand files it is the size question
  the design left open (single file or report folder), and it should be
  decided with real numbers.
- No HTML yet. The design's Phase 2 and 3 build the static page from this
  document; the engine's part was to produce it.
- `files` includes absent locators with `present: false`. A large, churning
  tree will accumulate them; retention is a policy the log does not have yet.
- The identity policy is whichever one is active. A report does not yet pin
  or carry several.
