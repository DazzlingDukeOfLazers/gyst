# Day 18 notes: running it on real projects

The first scan of something that was not a fixture. Twenty-three
repositories under one folder, 3.1 GB, scanned as one source into the
default SQLite store, then every repository's history walked and a
profile applied. Nineteen seconds to walk and hash 80,942 files; eighty
seconds all told. What came out, and what it changed.

## The first run

| | First run |
|---|---:|
| Files | 80,942 |
| Projects | 1,790 |
| Open findings | 15,321 |
| Files with multiple authority candidates | 50,790 |
| Coverage | partial: 108 entries could not be read |
| Report | 249 MB |

Every one of those numbers was wrong in an instructive way.

**1,790 projects.** Every `package.json` under `node_modules` is a project
marker. 1,745 of the projects lived inside dependency directories. The
discovery command already pruned those names when looking for roots; the
scanner emitting marker observations did not.

**15,321 findings.** 83 percent of the files were under `node_modules`,
`.venv`, `dist`, `build`, or a Python site-packages, and 12,476 of the
duplicate findings were entirely inside them. A copy in a dependency
cache is expected. Reporting it says nothing a person can act on, and it
buried the 233 duplicate groups that spanned two real projects.

**108 unreadable entries.** All 108 were symlinks. The scanner refuses to
follow them by design and then counted the refusal as a hole in
coverage, so the pass was partial and tombstones were disabled. Every
tree with a `node_modules/.bin` would have lost its deletion detection.

**Two manifests matched nothing.** The fixture's manifests inside the
gyst checkout declared members as `engineering/widget/**`, relative to
the scan root, and the scan root was three folders higher. A manifest
that means something different depending on where the scan starts is
not a manifest. Members are now relative to the manifest's own folder,
with a leading slash for the rare root-relative case.

**249 MB.** Eighty thousand single-file artifacts repeated what each
file's `grouping` entry already said.

## What changed

- Files under dependency, build, cache, and tool directories are
  *vendored*: observed like any other file, but not project markers, not
  duplicate findings, not authority candidates. The same name list the
  discovery command prunes. The report marks them `vendored: true`. Two
  names joined the list from this run: Godot's `.godot` and Claude Code's
  `.claude`, whose worktrees are whole copies of the repository they sit in.
- The vendored filter applies in the projection too, not only at the
  scanner, so a log written before the scanner knew does not keep
  producing dependency projects.
- Symlinks are counted and reported as not followed. They are not holes.
- Manifest member patterns are relative to the manifest's folder.
- Empty files are not authority candidates for each other, as they were
  already not duplicates.
- The report's `artifacts` section lists only groupings with more than
  one member.

## The last run

| | First run | Last run |
|---|---:|---:|
| Projects | 1,790 | 32 |
| Open findings | 15,321 | 1,412 |
| Duplicate groups spanning two projects | 233 | 232 |
| Files with multiple authority candidates | 50,790 | 4,116 |
| Coverage | partial | complete |
| Report | 249 MB | 169 MB |

The 32 projects are the 23 repositories, the fixture's two manifests and
firmware repository inside the gyst checkout, and a handful of real
nested markers: Godot projects inside game repositories, a worker and a
site inside the website, a resume configurator inside the job search.

The findings that remain are real. Sound patches from one project copied
into another's build output. A font licence copied into three projects.
Board folders in the hardware repository that carry copies of each
other's library files, which is exactly the situation the product was
described around. And two things that are noise of a kind Gyst should
not decide about: a scrape cache in one project with 816 unique-content
directories, and a build output folder named `dist-openai` that the
vendored list does not know. Both are what `.gystignore` is for, and the
right default is to leave them to the person who owns the tree.

## What this does not do

- The vendored list is a built-in convention, not policy. When
  `.gyst/policy.yaml` exists it should be able to extend or replace it.
- 1,412 findings is still a wall. The report needs the grouping the design
  describes, by project and by rule, before a person can read it. That is
  step two.
- Nothing here changed the scanner's handling of large files; the
  177 MB game binary and the 144 MB worker binary were hashed like
  anything else, in the time it took.
