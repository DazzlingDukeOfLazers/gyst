# Day 14 notes: portable spellings

Stage two of ADR 004. The statements that had a PostgreSQL-only spelling
now have one both engines accept, wherever that costs PostgreSQL nothing.
The store still speaks only to PostgreSQL; what changed is the SQL it
speaks.

## What was rewritten, and how

| Was | Is now |
|---|---|
| `DISTINCT ON (source, locator) ... ORDER BY seq DESC` | `WHERE seq = (SELECT max(seq) ... )` |
| `LEFT JOIN LATERAL (... ORDER BY started_at DESC LIMIT 1)` | join on `started_at = (SELECT max(started_at) ...)` |
| `payload->>'key'`, `(payload->>'valid')::boolean` | select the JSON column, decode in Go |
| `subjects @> $1::jsonb`, `jsonb_array_elements(subjects)` | list the few candidate rows, filter in Go |
| `waiver ? 'expires_at'` and a `CASE` inside the upsert | read the prior row, decide in Go, upsert with an explicit status |
| `WITH prior AS (...) INSERT ... RETURNING (SELECT ...)` | a `SELECT` then an upsert, in one transaction |
| `json_agg`, `array_agg`, `json_build_object` | three plain queries assembled in Go |
| `observed_at::text` as the pass key | format the timestamp in Go, identically on every engine |
| `(payload->>'last_known_seq')::bigint` join | decode the seq, then fetch digests with `IN (...)` in chunks |
| `now()` everywhere | the application clock, passed as a parameter |
| `TEXT[]` columns | JSON arrays (migration 0011) |

The array change is the only one with a migration. Every reader already
treated those columns as lists of strings and the report already emitted
them as JSON arrays; storing them that way means one spelling for the
write and the read, and a constraint that says the same thing in each
dialect. The driver marshals a `[]string` into a JSON column and back
without the Go code noticing.

## What remains PostgreSQL-only

Only what has to: the migrations. `BIGSERIAL`, `TIMESTAMPTZ`, `JSONB`, GIN
indexes, and the three PL/pgSQL triggers that make observations
append-only and assertions durable. Those get a SQLite twin in stage
three, where the triggers become SQLite triggers with `RAISE` and the
types become what SQLite has.

Timestamps are the other seam. PostgreSQL returns `time.Time`; SQLite will
return text or an integer, and the engine layer will have to convert at
the boundary. Every statement now passes and receives times as Go values,
so that conversion has one place to live.

## The check, again

Binary from `main` (stage one) against binary from this branch, `gyst
report` on the same database, no scan between: zero differences. Then
every write path on the development database: scan, Git walk, acknowledge,
waive with a past expiry and watch it reopen, assert, retract. Replay
verification on both databases.

## What the check found

An ambiguity finding listed the same candidate twice. Rename detection
ran on two passes and produced two compare-set relations pointing at the
same file, and the loader took each as a candidate. One file is one
candidate, and one candidate offered twice is not an ambiguity. The
detector now deduplicates by file, with a test.
