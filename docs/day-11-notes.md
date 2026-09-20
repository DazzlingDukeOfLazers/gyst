# Day 11 notes: expected projects in the fixture

The designer's first response to the handoff asked for two things before
report integration: hand-authored expected project membership in the
fixture, and one explicit case of a file in two projects. Both are in.

## A second manifest

The share now carries a root manifest, `engineering/.gyst/project.yaml`,
declaring project `engineering` over `engineering/**`. The widget manifest
beneath it is unchanged. Every widget file is therefore a member of both,
which is the many-to-many case the product is built around and the one a
tree view cannot show. Ten of the twenty inventoried files are in two
projects. The firmware repository has no manifest; its `.git` is a native
marker, so it is a suggestion at 0.6 and the fixture names it by folder,
`marker:firmware`, rather than by a derived id.

Each entry in `testdata/generate.py` now carries a `projects` list, written
by hand next to the file it describes, in the same way the `profiles` map
was written on day 1. The inventory also lists the declared projects and the
two folders whose `.gyst` marker is redundant because a manifest sits in
them.

## The check

`internal/project/fixture_test.go` parses every manifest in the generated
tree, lists every folder with markers, resolves projects through the same
function the projector uses, and matches every inventoried file against the
resulting patterns. It compares the set of projects, the suppressed-marker
count, and each file's membership against the inventory. It fails if the
fixture ever loses its two-project case.

This is the pattern the identity profiles established: the expectations are
an independent statement of what correct looks like, not a restatement of
what the code does. The live projection against a clean database agrees
with it: 18 files in `engineering`, 10 in `widget`, 2 in the marker project.

## Authority, restated

The designer's correction is right and worth repeating here. A manifest
declares a project and its membership. It does not declare which member is
the authority, and membership confidence must not be read as authority
confidence. The report carries no authority field. `files[].grouping` has an
`is_current` flag, which is the identity profile's notion of the current
member of a group; that is not authority either, and the report should not
present it as such. Authority needs its own assertion, and until it exists
the honest state is "no authority identified".
