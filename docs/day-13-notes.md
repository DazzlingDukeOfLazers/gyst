# Day 13 notes: the store boundary

Stage one of ADR 004. Every SQL statement now lives in `internal/store`,
one method per statement, and nothing else imports the database driver.
The pool accessor is gone, so the compiler enforces the boundary rather
than a convention.

## What moved

Sixteen files ran SQL. They now call about forty-five store methods,
grouped by domain: files and observations, the projection fold, identity,
relations, commits, projects, findings, authority, and the report's
inventory. Each method takes and returns plain Go structs. Transactions
that used to be opened by a projector and driven statement by statement
are now single store calls that take the whole unit of work: replace the
projects, write the commits and their relations, apply an identity plan.
The projectors compute in memory and hand over a result.

Two loaders were shared by four packages under four different spellings.
`PresentFiles` is now the one way to walk the inventory, and identity,
membership, findings, and authority all start from it.

## What the check was

The refactor claims no behaviour change. The check was to build the
binary from `main` and the binary from this branch, run `gyst report`
with each against the same database with no scan in between, and diff
the documents ignoring the clock. Zero differences. `gyst verify` still
replays exactly, every test passes, and every command was run on the
development database.

## What the check found

The first diff was not zero. Two confidences differed: the marker
project's 0.6 came out as 0.6000000238418579. The columns were `REAL`.
A float32 read into a float64 carries the artefact; Postgres hid it
wherever a value passed through `json_agg`, which prints float4 to six
digits, and showed it wherever a value was scanned directly. The old code
happened to use the first path for file memberships and the second for
everything else. Unifying the paths made the inconsistency visible.

Migration 0010 makes every confidence column double precision, rounding
existing values on conversion. That is what every reader expects and what
SQLite's `REAL` already is, so the two engines will agree by construction
rather than by formatting accident.

## What this does not do

- Nothing is portable yet. The statements moved; they did not change.
  `DISTINCT ON`, `LATERAL`, JSONB operators, and `TEXT[]` are all still
  there, now in one place where stage two can see them.
- `LoggedFile` and `Since` predate the boundary and keep their shape.
- The store has no tests of its own. The projectors' tests are pure and the
  commands are checked live; a store test suite is what stage three needs
  to run the same assertions against both engines.
