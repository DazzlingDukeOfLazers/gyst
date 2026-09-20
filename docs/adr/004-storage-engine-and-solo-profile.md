# ADR 004: Storage engine and the solo profile

Status: accepted, 2026-09-20, by Daniel.

## Context

Two of the constitutional constraints in `docs/strategy.md` say that core
operation requires no server and that a clean offline installation can
inventory a project and explain its results. The round's third goal was
that a diaspora can pull the repository, build it, and run their own Gyst.

Today none of that is true of the code. `gyst` opens `postgres:///gyst` and
nothing works without a running PostgreSQL. A contributor who clones the
repository must install and start a database server before the first
scan. This session is the proof: the project had never been built on this
Mac, and getting to a first scan meant installing Go and PostgreSQL and
starting the server by hand.

The architecture record anticipated this: "SQLite may support the solo
profile, but PostgreSQL remains the reference behavior." This ADR decides
how.

### What the code depends on now

| Measure | Count |
|---|---|
| Files that run SQL against the pool | 16 |
| Query sites (`Query`, `QueryRow`, `Exec`, batches) | 89 |
| Statements using `ON CONFLICT` upserts | 16 |
| Statements using `DISTINCT ON` | 3 |
| Statements using `LATERAL` joins | 3 |
| Statements using JSONB operators (`->>`, `@>`, `?`) or aggregates (`json_agg`, `string_agg`) | 9 |
| Columns typed `TEXT[]` | 6 |
| PL/pgSQL trigger functions (append-only observations, durable assertions) | 3 |
| Places that import `pgx` directly | 9 |

Roughly fifteen statements would need an engine-specific variant. The rest
is portable SQL. The larger cost is structural: SQL is written inline in
every package, so there is no single place an engine boundary could sit.

## Options

### A. SQLite as a second engine behind a store boundary

Move every statement into `internal/store` behind a small engine interface,
then add SQLite alongside PostgreSQL. `modernc.org/sqlite` is a pure-Go
translation of SQLite: no C toolchain, cross-compiles to Windows with
`GOOS=windows`, BSD-3 licensed, adds roughly 8 MB to the binary. SQLite has
what the divergent statements need in a different spelling: triggers with
`RAISE`, `json_extract` and `json_each` for the JSONB uses, window functions
or a correlated `max(seq)` for `DISTINCT ON`, correlated subqueries for
`LATERAL`, JSON text for arrays, `INSERT ... ON CONFLICT` for upserts.

Cost: two dialects to keep correct. Mitigated by running every test and the
fixture report against both engines and asserting the reports are equal
apart from timestamps.

### B. SQLite only

Drop PostgreSQL. One dialect, the simplest code, and the solo profile is the
only profile. It reverses a recorded decision: the team and air-gapped
profiles expect a central server with several agents appending
concurrently, and SQLite serialises writers. A hundred thousand files from
one agent is fine; a dozen agents is where it stops being fine.

### C. Embedded PostgreSQL

`embedded-postgres` runs a real PostgreSQL inside the process. It downloads
the server binaries at first run, which the offline constraint forbids
outright. Bundling them instead adds about 30 MB per platform, a second
process to supervise, and a build that ships binaries Gyst did not compile.
Verifiable-binary and SBOM promises get harder to keep.

### D. Keep PostgreSQL, document the install

Costs nothing now and fails the goal. Every contributor pays the server
setup, every demo needs a service, and the "no server required" promise
stays untrue.

## Decision (proposed)

Option A, in four stages, each a pull request that leaves the tree working:

1. **Draw the boundary.** Move all SQL from the sixteen files into
   `internal/store`, one method per statement, with no behaviour change.
   `gyst verify` and a byte-equal fixture report before and after are the
   check. This stage has value on its own: it is where the pass-id and
   retention work will also need to sit. *Done 2026-09-20; see
   `docs/day-13-notes.md`. Old and new binaries produce identical reports
   on the same database.*
2. **Portable where cheap.** Rewrite statements that have a portable
   spelling with no cost on PostgreSQL. Arrays become JSON text; `now()`
   moves to the application clock, which the pass work already began.
   *Done 2026-09-20; see `docs/day-14-notes.md`. No dialect-specific SQL
   remains in the store; only the migrations are PostgreSQL's.*
3. **Add the SQLite engine.** The fifteen divergent statements get a
   variant. Migrations get a second directory or a dialect switch. Both
   engines run the whole test suite and produce an equal fixture report.
   *Done 2026-09-20; see `docs/day-15-notes.md`. No variants were needed
   after stage two; the store runs on database/sql with one engine object
   for placeholders, times, and JSON. The fixture report is byte-identical
   across engines, and a store suite runs on both.*
4. **Make it the default.** With `GYST_DATABASE_URL` unset, open a SQLite
   file under the user's data directory. PostgreSQL remains the reference
   engine for the team profile and stays in the documentation as such.

## Acceptance

- On a clean machine with Go installed and no services running: clone,
  `go build ./cmd/gyst`, `gyst scan --root .`, `gyst report`. Nothing else.
- `go test ./...` passes on both engines.
- The fixture report from each engine is identical apart from `generated_at`.
- `gyst verify` reproduces the projection on both.
- A scan of a hundred thousand files completes on SQLite without unbounded
  memory, and the measured time is recorded in the day notes. *Measured
  2026-09-20: 36 s full, 22 s incremental, 134 MB file; day-15 notes.*

## Consequences

- One new dependency, `modernc.org/sqlite`, to be accepted under the
  dependency policy that ADR 002 still owes. It is BSD-3 and has no C.
- A store boundary that every later feature must respect: no SQL outside
  `internal/store`. That is a discipline the code does not have today and
  the first stage imposes it.
- Two engines to test forever. The fixture-report equality check is what
  makes that affordable.
- Estimated effort: three to five working sessions. Stage 1 is the largest
  and the least risky.

## Not decided here

Event compaction and retention, which ADR 004 as listed in `decisions.md`
also names. Absent locators accumulate in `current_files` and every
`file.metadata` re-observation stays in the log forever. That is a policy
question with the same shape on either engine and deserves its own record.
