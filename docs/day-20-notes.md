# Day 20 notes: reads that wrote, and bookkeeping retention

Two small things, both from the last two days.

## A report is a read

Generating a report re-ran finding detection first, so that a source
that had gone stale since the last scan would show as stale. That made
`gyst report` and `gyst findings` writes, and it bit on day 19: a report
generated with a binary that lacked the vendored filter quietly reopened
twelve thousand findings in a store it did not own. Both are reads now.
Findings are as of the last scan and the commands say so; the freshness
state in the report is still computed at generation from the pass record
and the clock, which touches nothing.

## Bookkeeping retention

The part of ADR 005 that touches no evidence, on by default at the end of
every scan: a source keeps its newest thirty passes and its very first,
and resolved findings older than ninety days are removed unless they carry
a waiver, which is a person's decision and is kept whatever its age. The
two constants live in the command, not the store. Nothing a finding,
relation, or release cites is affected, and the store suite proves the
first and newest passes survive and a waived finding does.

Compaction of the log itself, the other half of the record, waits until
someone needs it, as the record says.
