# Day 23 notes: one judgment through the whole system

The designer's reframing on day 21 was that a report a person can only
inspect is not yet a product, and that before any more presentation work
one thin loop should run end to end: see what needs judgment, judge one
candidate, watch the projection change, undo it. This is that loop, on
the command line, with the report kept read-only.

## The loop

`gyst review` is the queue. It says how many project records exist and
how many are declared, confirmed, ignored, or candidates, then lists the
candidates in the order the design asked for: nested boundaries first,
then cross-project findings, then open findings, then generic names, then
size. On the real tree that puts a resume configurator inside the job
search at the top, then the tools and mods inside the games, then the
repositories with the most copied files between them.

`gyst review <candidate>` explains one: where its boundary is, what it
is physically inside, why the record exists, its evidence, how many of
its files are vendored, how many findings it has, and then, in plain
words, what confirming or ignoring it would change and what would not.
Both commands are reads.

`gyst assert project confirm|ignore <id> --by <you> --reason "..."`
records the judgment. It refuses a declared record, since a manifest
already spoke, and refuses to judge a record twice, since the first
judgment must be retracted to be replaced, so every change is in the
history. Then it rebuilds membership, findings, and authority, and prints
the new counts, the sentence "nothing renamed, moved, or deleted", and
the exact retract command.

`gyst assert retract` withdraws it. The retraction is recorded on the
row; the marker suggestion stands again; the memberships an ignore had
withdrawn come back.

## What the schema needed

The assertions table gained a subject kind, so a subject can be a
project record, and two kinds, `project.confirm` and `project.ignore`.
Its edit-forbidding trigger covers the new columns. On SQLite that meant
rebuilding the table, since a check constraint cannot be altered in
place; the delta migration copies the rows across and recreates the
triggers. Projects gained a state: declared, candidate, confirmed,
ignored. Nothing about markers or manifests changed; the projection reads
the assertions last, because a person's word sits above both.

An assertion about a record that no longer exists, a folder moved or a
source gone, is counted as stale and applied to nothing. It is not
deleted and not reattached.

## What the report and page carry

`projects[].state`, four `projects_*` counts, and `assertions[]` with
`subject_kind`, `object`, and `value`. The page leads Projects with the
summary the design wrote, "We found N possible projects", with reviewed
and remaining counts, opens on the Needs review filter, marks each row's
state in words, and in the drawer shows the exact commands to record a
judgment and an Audit list of the record's assertions. It says plainly
that it cannot record the judgment itself.

## Proved

On the fixture: queue, explain, confirm, refuse a second judgment,
retract, ignore with memberships withdrawn, retract with memberships
restored, audit showing both retractions with who and why. The tree's
digest before and after is identical, a rescan reads nothing, and replay
verifies. Assertions are durable on both engines by the store suite.

## What this does not do

- Alias, containment, same-project, and member include and exclude are
  the vocabulary proposed in the handoff and are not built. They reuse
  this loop.
- Review later is the absence of an assertion; there is no way to mark a
  candidate as seen without judging it.
- The page previews the commands; it does not run them. A write path
  from an interactive client is the design's Stage C.
- The queue is recomputed from the report document each time, which is
  two seconds on the real store. Fine for a command; a client should
  keep the index.
