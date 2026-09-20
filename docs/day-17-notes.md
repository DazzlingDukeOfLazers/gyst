# Day 17 notes: where a hundred thousand files actually spend their time

The day-15 benchmark left a number on the table: 36 seconds for a full
scan of a hundred thousand files on SQLite, 22 for an incremental one that
read nothing, and a note saying the row-by-row inserts that replaced the
PostgreSQL batch API were the obvious cost. That note was a guess. This
is what measuring found.

## The guess, tried

Multi-row `INSERT ... VALUES (...),(...)` is portable and removes a round
trip per row, so every bulk writer was converted to it, chunked under both
engines' parameter limits. The scan got no faster on PostgreSQL and
twenty times slower on SQLite: 735 seconds for the full scan, 214 for the
incremental. The pure-Go SQLite driver's cost per statement grows much
faster than linearly with the number of parameters.

So the shapes were measured in isolation, twenty thousand observations
appended and folded:

| Rows per statement | SQLite append | SQLite fold | PostgreSQL append | PostgreSQL fold |
|---:|---:|---:|---:|---:|
| 1, prepared | 0.34 s | 0.19 s | 1.47 s | 2.01 s |
| 20 | 2.07 s | 0.32 s | 0.86 s | 1.07 s |
| 100 | 9.95 s | 1.46 s | 0.82 s | 1.41 s |
| 500 | 52.11 s | 7.52 s | 0.88 s | 1.56 s |
| 750 | 76.16 s | 11.81 s | 0.93 s | 2.03 s |

The engines want different things. SQLite wants one prepared single-row
statement executed per row inside the transaction. PostgreSQL halves its
time at about twenty rows per statement and gains nothing past that. So
the rows-per-statement is a property of the engine, set when it opens, and
the bulk writer honours it. The measurement is kept as a test that runs
only when asked, so the numbers can be reproduced when a driver changes.

And with that in place the scan was still 35 and 26 seconds. The inserts
had never been the cost.

## The cost, measured

Two candidates were timed alone at the benchmark's shape: the authority
resolver over a hundred thousand files with five thousand identical, and
the membership matcher over the same files against twenty-nine patterns.

| | Time |
|---|---:|
| Membership matching, 2.9 million glob tests | 0.42 s |
| Authority resolution | 18.91 s |

The resolver built each file's peer set as a map, then sorted it. For the
five thousand files sharing one digest, that was five thousand sorts of
five thousand keys. Every digest group and every identity group is now
sorted once, and a file's peers are the merge of two sorted lists, which
is linear. The same function now takes 1.68 seconds.

## Numbers

| Step | SQLite, day 15 | SQLite, now | PostgreSQL, now |
|---|---:|---:|---:|
| Full scan of 100,000 files | 36 s | 17.5 s | 20.6 s |
| Incremental rescan, nothing changed | 22 s | 4.9 s | 5.9 s |

Walking and hashing are about ten seconds of the full scan on either
engine and are the floor until the scanner reads files concurrently. The
incremental rescan is now mostly the projections rebuilding from a
hundred thousand rows, which is what they are for.

Also fixed on the way: the scanner's per-pass cap counted folder
observations, so a tree of exactly the cap came out partial by the number
of its repositories. It counts files now.

## What this does not do

- The scanner hashes one file at a time. Ten seconds of the full scan is
  that, and it is the next floor to lower.
- The projections rebuild everything every pass. Incremental membership
  and authority are possible and would take the rescan below a second;
  they are also where staleness bugs live, so they wait for a reason.
