# Day 22 notes: two contract additions from the curation design

The designer's project inventory design asked four contract questions.
These are the two that need no schema change, built first as promised.

## Boundaries and physical nesting

Every project record now carries `boundary`: the folder that holds its
manifest or carries its marker, with the source it is in. From the
boundaries alone, `physically_within` names the nearest other record in
the same source whose boundary is a strict path prefix. It is computed
from two locators and nothing else, and the field name says what it is.
Two `godot` records are now `godot inside 2Caves2Qud` and `godot inside
raves-of-qud` without opening either. Containment between projects, the
organisational claim, is an assertion and will be a separate field.

On the real tree eleven records are physically inside another: the two
Godot projects, a mod and a tool inside a game, a worker inside a
service, a resume configurator inside a job search with an archive
inside that, and the fixture's own manifests inside the gyst checkout.

## Findings know their projects

Every finding carries `projects`, the union of the memberships of its
file subjects, and `cross_project` when they span more than one. The
page groups findings by that now instead of by the first path segment of
the first subject, which was a placeholder that the design rightly
called out. On the real tree 244 of 1,412 open findings cross a project
boundary, and the largest pair is get-in-loser and miniPaint sharing 146
copied icon files.

Subjects that are not files, the source root of a stale-source finding,
contribute nothing, so a finding's project list may be empty and the
page says "no project" rather than inventing one.

## What this does not do

The other two questions wait: cited observations embedded in the report,
and the project-curation assertions whose vocabulary is proposed in the
handoff log for the designer and Daniel to agree.
