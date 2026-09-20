# Day 8 notes: projects and membership

The product's central claim is that a project is not a folder. Until today
the code had no projects at all, only folders, and the package called
`project` was the projector. This adds projects as a projection over two
kinds of evidence, with the precedence the round-two design set down:
a checked-in manifest declares, a native marker suggests.

## The manifest

`.gyst/project.yaml` is the format the fixture already used:

```yaml
name: Widget
members:
  - engineering/widget/**
```

`id` may be given and is otherwise derived from the name. `members` are glob
patterns relative to the source root, matched against locators, with `**`
spanning directories and a trailing `/**` meaning inside the folder rather
than the folder itself, as in `.gitignore`. A manifest with no members
claims its own folder. Unknown keys warn rather than reject, so a newer
manifest stays readable by an older Gyst. A manifest that cannot be parsed
is still observed, with `valid: false` and the error, so a broken manifest
is visible in `gyst explain` rather than silently absent.

The manifest is read under any content level except `exclude`. That is a
policy decision worth stating: it is not engineering data crossing a
boundary, it is the owners of the tree describing the tree to Gyst, and a
policy that forbade reading it would forbid the project from ever being
declared. Under `metadata` the manifest observation carries no digest and
the schema constraint still holds.

## Two claims about one locator

The manifest file already had a `file.content_fingerprint` observation. It
now has a `project.manifest` one as well, a second claim about the same
locator with a different extractor and therefore a different id. That
exposed an assumption in the projector: `current_files` folded every
file-kind observation, and the manifest claim, carrying no digest under
metadata policy, would have overwritten the fingerprint that did. The fold
is now limited to the three claims that state a file's condition on disk.

The manifest is also re-read on incremental passes, when the file itself is
skipped as unchanged. Its observation derives the same id from the same
version, so the log deduplicates it. What the re-read buys is that a
manifest first seen by an older scanner, or observed before the projector
existed, cannot go missing. The fixture found this: the first rescan after
the feature landed produced no project, because the manifest was known and
unchanged.

## Markers

A directory carrying `.git`, `go.mod`, a KiCad project file, or one of a
dozen other markers gets a `folder.metadata` observation listing them. The
markers are facts at confidence 1.0. The projection turns each marked
folder into a project at 0.6, named for the folder, with the explanation
that a marker suggests a boundary but does not declare one. Below the 0.8
line, so it reads as a suggestion everywhere confidence is shown.

A marker in the same folder as a valid manifest is dropped as redundant
rather than recorded as a second project: the manifest has named what the
marker only hinted at. A marker elsewhere is not dropped even when a
manifest's patterns cover it. The fixture's `firmware/` repository sits
beside the widget project, not inside it, and a nested repository inside a
declared project would legitimately be both.

## One id, several sources

Two manifests with the same id in different sources describe one project.
That is the mechanism by which a project spans a repository and a share,
and the test for it is the one that matters most in this change.

## What this does not do

- No explicit assertions yet. A person cannot say "this file is in that
  project" or "not that one". That is the top of the precedence order and
  the reconciliation flow the design describes.
- No organization rules and no suggestions from content or history.
- `member-of-project` relations are not written; membership lives in its own
  tables. The relation schema has the type and the examples do not yet use it.
- Folders get no tombstones, so a marker folder is taken to be gone only when
  nothing is present beneath it.
- `expected-inventory.json` does not yet state expected projects. The fixture
  generator should declare them so the projection can be checked against a
  hand-authored expectation, the way identity groupings are.
