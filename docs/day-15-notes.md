# Day 15 notes: the SQLite engine

Stage three of ADR 004. The store now runs on either PostgreSQL or a
SQLite file, and the same recipe on both produces the same report.

## One body of code, two drivers

The store moved from the PostgreSQL driver's own API to Go's standard
`database/sql`, which both drivers implement. Everything engine-specific
lives in one small object that every statement passes through. It does
three things and nothing else:

- Rewrites `$N` placeholders to `?N` for SQLite, which binds argument N
  wherever it appears and however often, the way PostgreSQL does.
- Converts arguments: a `time.Time` becomes fixed-width UTC text with
  microseconds on SQLite, so text order is time order and precision
  matches what PostgreSQL keeps; a `[]string` becomes a JSON array; JSON
  bytes become text so PostgreSQL does not take them for `bytea`.
- Opens the right driver from the DSN, and on SQLite applies the embedded
  schema, sets foreign keys on for the cascades the projections rely on,
  and uses write-ahead logging so a report can read while a scan writes.

On the read side, three small scanner helpers do the reverse: a
timestamp from either driver's representation, a nullable timestamp, and
a JSON array into a `[]string`. They sit at the scan sites and nowhere
else.

The SQL itself did not change. Stage two had already made it portable,
and the only spellings that differed were in the migrations. SQLite gets
one consolidated schema, embedded in the binary and applied on first
open, with its own triggers saying what the PL/pgSQL ones say: no update
or delete on observations, no delete or edit on assertions.

`GYST_DATABASE_URL=sqlite:gyst.db` is all it takes. No server, no
service, no client tools. The default is still PostgreSQL; flipping it is
stage four.

## The check

Three checks, each stricter than the last.

First, the old check: the binary from `main` and the binary from this
branch against the same PostgreSQL database. The only difference is the
order of lists, and that is deliberate: the report now sorts every list
in Go by byte order, because databases sort text by collation and
collations differ between engines and between machines. The report is a
contract and must come out the same everywhere.

Second, the ADR's own acceptance test: the fixture recipe run from empty
on PostgreSQL and on a SQLite file, and the two reports compared. After
stripping clocks and the two ids derived from clocks, the documents are
byte-identical: 37,450 characters, no difference.

Third, a store test suite that runs the same assertions on both engines,
covering every method: the log and its immutability, the projection and
its replay, passes, sources, identity plans, relations and their
constraints, commits, tombstones and arrivals, projects, findings through
their whole lifecycle, assertions and their durability, authority. SQLite
always runs on a temporary file. PostgreSQL runs when
`GYST_TEST_DATABASE_URL` names a disposable database, which the suite
resets from the migration files.

## What the suite found

A real defect, and both engines agreed on it. Re-detecting a finding
that is waived and not yet expired failed the `waived_requires_waiver`
check. Stage two had moved the status decision into Go and passed the
computed status into the inserted row; a proposed row saying waived with
no waiver is rejected before the conflict branch runs, on either engine.
The old CTE version had always inserted `open`. The upsert now does that
again and applies the computed status only in the update branch, where
the row carries its waiver. The live check on day 14 had only exercised
the expired-waiver path, which reopens and so never trips the constraint.
The suite tests the path that matters.

## Numbers

A synthetic tree: 100,000 files of 64 random bytes in 200 folders, 29 of
them carrying a `.git` marker, five percent of the files byte-identical to
each other. Scanned on a SQLite file on this laptop:

| Step | Time | Note |
|---|---|---|
| Full scan | 36 s | 11.5 s walking and hashing; the rest is the projections |
| Incremental rescan, nothing changed | 22 s | no file read; membership, authority, and findings rebuilt |
| Report | 1.6 s | 80 MB of JSON for 100,000 files |
| Database file | 134 MB | |
| Replay verification | passes | |

The projections dominate, and within them the row-by-row inserts that
replaced the PostgreSQL driver's batch API. That is the next thing to
measure and the obvious place to win back time; it is not a correctness
concern and the memory stays bounded.

## What the benchmark found

The first run produced a 1.5 GB database and a 1.8 GB report, and a
report that took a minute. The cause was quadratic evidence. Five thousand
files shared one digest, so each of them resolved to "multiple candidates"
citing all five thousand peers, and the one duplicate finding listed all
five thousand subjects. The rename detector had already capped its
candidates at four for exactly this reason: beyond a handful the list
stops being a question a person can answer.

Authority now names and cites at most five peers and says "and N more";
the explanation still carries the true count. A duplicate finding names
at most twenty-five subjects, with the true count in the summary. Both
have tests. The database shrank twelvefold and the report twentyfold.

## What this does not do

- PostgreSQL is still the default. Stage four.
- The Git connector still shells out to `git`; that is unrelated to the
  store but is the other thing a clean machine needs.
- Batch inserts are loops now. PostgreSQL's driver had a batch API that
  saved round trips; the standard interface has none. At the fixture's
  size this is invisible; at a hundred thousand files it is most of the
  36 seconds above. Multi-row inserts are portable and are the fix.
- The scanner's default cap of 100,000 observations per pass counts
  folder observations too, so a tree of exactly 100,000 files comes out
  partial by 29. `--max-files` raises it; the default should count files.
