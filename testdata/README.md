# Synthetic messy dataset

Backlog item 3. A deliberately awkward engineering tree plus the inventory a
correct scanner should produce from it.

The tree is generated, not committed. `generate.py` is the authority; the tree is
disposable.

```bash
python3 testdata/generate.py           # build tree/ and expected-inventory.json
python3 testdata/generate.py --check   # rebuild in a temp dir and prove determinism
```

## Why the expectations are hand-authored

`FILES` in `generate.py` declares both the bytes and the interpretation a scanner
should reach — which files are generated, which are duplicates, how each identity
profile should group them. Only mechanical facts are computed: digests, sizes,
and commit ids.

A fixture whose expectations were derived by the same code that built the tree
would restate the generator's assumptions and confirm nothing. The expectations
have to be an independent claim, or the fixture cannot catch a wrong one.

## Determinism

Content is fixed, mtimes are pinned, and the Git repository is built with fixed
author, committer, and dates, so commit ids are stable across machines. The day 2
scan benchmark is meaningless against a tree that shifts underneath it, and
`--check` fails loudly if anything becomes non-reproducible.

## What each case is for

| Case | Tests |
|---|---|
| `widget_rev2.pdf` / `widget_rev3.pdf` | `suffix-as-version` grouping and supersession. |
| `connector_123.pdf` / `connector_124.pdf` | **The trap.** Under `suffix-as-version` these wrongly collapse and one appears superseded. They are distinct part numbers with no revision relationship. |
| `Assembly Notes v2.pdf` / `... v2 FINAL.pdf` | Genuine ambiguity. `FINAL` is not a revision scheme, so confidence must fall below the supersedes threshold and the result must be a compare-set. |
| `widget_bom.xlsx` / `widget_bom (copy).xlsx` | Byte-identical content. Duplicate detection must be content-derived; the filename is corroboration, not the basis. |
| `output/*.gbr`, `*.drl`, `generation.log` | Generated outputs as first-class artifacts, including the log itself. |
| `désign-notes.md` | Non-ASCII path. macOS and Windows differ on NFC/NFD normalisation; one file must not become two. |
| `empty.txt` | Zero bytes. Hashes to the well-known empty digest and must not be reported as a duplicate of every other empty file. |
| `scratch/tmp-ignore-me.log` | `.gystignore` exclusion. Must not appear in the inventory at all, not even path-only. |
| `firmware/` | A real Git repository with two commits, for native provenance. |

## Not represented yet

- A file actively being written, for the stability check before hashing. This
  needs a live writer rather than a static tree.
- Locked files, long paths, and network shares — all Windows-specific and out of
  reach on a macOS host.
- Case-insensitive collisions (`Widget.pdf` versus `widget.pdf`), which cannot
  coexist in one tree on a case-insensitive volume and need a separate fixture.
