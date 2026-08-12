#!/usr/bin/env python3
"""Build the synthetic messy engineering dataset and its expected inventory.

The tree is regenerated rather than committed, so that binary fixtures do not
bloat history and so that every byte is explained by the declaration below.

The split matters. FILES is hand-authored and carries the *interpretation* a
correct scanner should reach: which files are generated, which are duplicates,
and how each identity profile should group them. Everything the generator emits
alongside it -- digests, sizes, commit ids -- is mechanical. A fixture that
derived its own expectations from its own generator would prove nothing.

Determinism is required: identical content, fixed mtimes, and pinned Git author
and committer dates, so commit ids are stable across machines and reruns. The
scan benchmark on day 2 is meaningless if the tree shifts underneath it.

Usage:
    python3 testdata/generate.py            # build tree/ and expected-inventory.json
    python3 testdata/generate.py --check    # rebuild in a temp dir and diff
"""

import argparse
import hashlib
import json
import os
import shutil
import subprocess
import sys
import tempfile
import zipfile
from pathlib import Path

HERE = Path(__file__).resolve().parent
TREE = HERE / "tree"
EXPECTED = HERE / "expected-inventory.json"

# Fixed clock. Any value works; it only has to never change.
EPOCH = 1723420800  # 2024-08-12T00:00:00Z
GIT_DATE = "2024-08-12T00:00:00+00:00"
GIT_AUTHOR = ("Gyst Fixture", "fixture@gyst.invalid")


# --------------------------------------------------------------------------
# Minimal real binaries. These carry correct magic bytes and structure so that
# file-type detection and later extractor work face something honest.
# --------------------------------------------------------------------------

def make_pdf(text: str) -> bytes:
    """A small but structurally valid PDF with a real cross-reference table."""
    objs = [
        b"<</Type/Catalog/Pages 2 0 R>>",
        b"<</Type/Pages/Kids[3 0 R]/Count 1>>",
        b"<</Type/Page/Parent 2 0 R/MediaBox[0 0 300 200]/Contents 4 0 R"
        b"/Resources<</Font<</F1 5 0 R>>>>>>",
        None,  # content stream, built below
        b"<</Type/Font/Subtype/Type1/BaseFont/Helvetica>>",
    ]
    stream = b"BT /F1 12 Tf 20 100 Td (" + text.encode("ascii", "replace") + b") Tj ET"
    objs[3] = b"<</Length " + str(len(stream)).encode() + b">>stream\n" + stream + b"\nendstream"

    out = bytearray(b"%PDF-1.4\n")
    offsets = []
    for i, body in enumerate(objs, start=1):
        offsets.append(len(out))
        out += str(i).encode() + b" 0 obj" + body + b"endobj\n"

    xref_at = len(out)
    out += b"xref\n0 " + str(len(objs) + 1).encode() + b"\n"
    out += b"0000000000 65535 f \n"
    for off in offsets:
        out += ("%010d 00000 n \n" % off).encode()
    out += b"trailer<</Size " + str(len(objs) + 1).encode() + b"/Root 1 0 R>>\n"
    out += b"startxref\n" + str(xref_at).encode() + b"\n%%EOF\n"
    return bytes(out)


def make_xlsx(rows) -> bytes:
    """A minimal but genuinely openable .xlsx holding one sheet of BOM rows."""
    def cell(ref, value):
        if isinstance(value, (int, float)):
            return f'<c r="{ref}"><v>{value}</v></c>'
        esc = str(value).replace("&", "&amp;").replace("<", "&lt;")
        return f'<c r="{ref}" t="inlineStr"><is><t>{esc}</t></is></c>'

    sheet_rows = []
    for r, row in enumerate(rows, start=1):
        cells = "".join(cell(f"{chr(65 + c)}{r}", v) for c, v in enumerate(row))
        sheet_rows.append(f'<row r="{r}">{cells}</row>')
    sheet = (
        '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
        '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main">'
        f'<sheetData>{"".join(sheet_rows)}</sheetData></worksheet>'
    )
    parts = {
        "[Content_Types].xml":
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
            '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
            '<Default Extension="xml" ContentType="application/xml"/>'
            '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>'
            '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>'
            "</Types>",
        "_rels/.rels":
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>'
            "</Relationships>",
        "xl/workbook.xml":
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
            'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
            '<sheets><sheet name="BOM" sheetId="1" r:id="rId1"/></sheets></workbook>',
        "xl/_rels/workbook.xml.rels":
            '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>'
            '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
            '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>'
            "</Relationships>",
        "xl/worksheets/sheet1.xml": sheet,
    }

    import io
    buf = io.BytesIO()
    # Fixed date_time and sorted names keep the archive byte-identical per run.
    with zipfile.ZipFile(buf, "w", zipfile.ZIP_DEFLATED) as z:
        for name in sorted(parts):
            info = zipfile.ZipInfo(name, date_time=(2024, 8, 12, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            z.writestr(info, parts[name])
    return buf.getvalue()


BOM_ROWS = [
    ["Item", "MPN", "Qty", "Ref"],
    ["ITM-1001", "RC0603FR-0710KL", 4, "R1,R2,R3,R4"],
    ["ITM-1002", "GRM188R71C104KA01D", 2, "C1,C2"],
    ["ITM-1003", "STM32G031K8T6", 1, "U1"],
]

GERBER = (
    "%FSLAX46Y46*%\n%MOMM*%\n%ADD10C,0.500000*%\n"
    "D10*\nX1000000Y1000000D02*\nX2000000Y1000000D01*\nM02*\n"
)


# --------------------------------------------------------------------------
# The dataset declaration. Expectations here are hand-authored on purpose.
#
#   generated_from  -> the source this output should be attributed to
#   duplicate_of    -> identical content expected under a different name
#   ignored         -> .gystignore must keep this out of the inventory entirely
#   profiles        -> expected grouping key per identity profile. Files sharing
#                      a key under a profile are expected to group together.
# --------------------------------------------------------------------------

FILES = [
    # --- suffix-as-version: a supplier revision drop -----------------------
    {
        "path": "engineering/widget/widget_rev2.pdf",
        "content": make_pdf("WIDGET REV 2"),
        "profiles": {
            "content-path-exact": "engineering/widget/widget_rev2.pdf",
            "suffix-as-version": "engineering/widget/widget",
            "suffix-as-identity": "engineering/widget/widget_rev2",
            "compare-set": "engineering/widget/widget:set",
        },
        "note": "Superseded by rev3 under suffix-as-version only.",
    },
    {
        "path": "engineering/widget/widget_rev3.pdf",
        "content": make_pdf("WIDGET REV 3"),
        "profiles": {
            "content-path-exact": "engineering/widget/widget_rev3.pdf",
            "suffix-as-version": "engineering/widget/widget",
            "suffix-as-identity": "engineering/widget/widget_rev3",
            "compare-set": "engineering/widget/widget:set",
        },
        "note": "Current under suffix-as-version; a distinct artifact under suffix-as-identity.",
    },

    # --- duplicate content under a different name --------------------------
    {
        "path": "engineering/widget/widget_bom.xlsx",
        "content": make_xlsx(BOM_ROWS),
        "profiles": {
            "content-path-exact": "engineering/widget/widget_bom.xlsx",
            "suffix-as-version": "engineering/widget/widget_bom",
            "suffix-as-identity": "engineering/widget/widget_bom",
            "compare-set": "engineering/widget/widget_bom:set",
        },
    },
    {
        "path": "engineering/widget/widget_bom (copy).xlsx",
        "content": make_xlsx(BOM_ROWS),
        "duplicate_of": "engineering/widget/widget_bom.xlsx",
        "profiles": {
            "content-path-exact": "engineering/widget/widget_bom (copy).xlsx",
            "suffix-as-version": "engineering/widget/widget_bom (copy)",
            "suffix-as-identity": "engineering/widget/widget_bom (copy)",
            "compare-set": "engineering/widget/widget_bom:set",
        },
        "note": "Byte-identical to widget_bom.xlsx. Duplicate detection must be "
                "content-based; the name gives no usable signal.",
    },

    # --- generated outputs --------------------------------------------------
    {
        "path": "engineering/widget/output/widget-F_Cu.gbr",
        "content": GERBER.encode(),
        "generated_from": "engineering/widget/widget.kicad_pcb",
    },
    {
        "path": "engineering/widget/output/widget-B_Cu.gbr",
        "content": GERBER.replace("X2000000", "X3000000").encode(),
        "generated_from": "engineering/widget/widget.kicad_pcb",
    },
    {
        "path": "engineering/widget/output/widget.drl",
        "content": b"M48\nMETRIC,TZ\nT1C0.300\n%\nG90\nT1\nX10.0Y10.0\nM30\n",
        "generated_from": "engineering/widget/widget.kicad_pcb",
    },
    {
        "path": "engineering/widget/output/generation.log",
        "content": b"kicad-cli pcb export gerbers --output output/ widget.kicad_pcb\nOK\n",
        "generated_from": "engineering/widget/widget.kicad_pcb",
        "note": "The log is itself an artifact of the generation run, not a stray file.",
    },
    {
        "path": "engineering/widget/widget.kicad_pcb",
        "content": b"(kicad_pcb (version 20240108) (generator pcbnew)\n  (general (thickness 1.6))\n)\n",
        "note": "Source of the output/ directory. Indexed as a file; no semantic "
                "parse until the KiCad extractor exists.",
    },

    # --- suffix-as-identity: part numbers, not revisions --------------------
    {
        "path": "engineering/connectors/connector_123.pdf",
        "content": make_pdf("CONNECTOR 123"),
        "profiles": {
            "content-path-exact": "engineering/connectors/connector_123.pdf",
            "suffix-as-version": "engineering/connectors/connector",
            "suffix-as-identity": "engineering/connectors/connector_123",
            "compare-set": "engineering/connectors/connector:set",
        },
        "note": "THE TRAP. Under suffix-as-version this wrongly collapses with "
                "connector_124 and one appears superseded. 123 and 124 are "
                "distinct parts; no revision relationship exists.",
    },
    {
        "path": "engineering/connectors/connector_124.pdf",
        "content": make_pdf("CONNECTOR 124"),
        "profiles": {
            "content-path-exact": "engineering/connectors/connector_124.pdf",
            "suffix-as-version": "engineering/connectors/connector",
            "suffix-as-identity": "engineering/connectors/connector_124",
            "compare-set": "engineering/connectors/connector:set",
        },
    },

    # --- genuinely ambiguous: must fall back to compare-set -----------------
    {
        "path": "engineering/vendor-drop/Assembly Notes v2.pdf",
        "content": make_pdf("ASSEMBLY NOTES V2"),
        "profiles": {
            "content-path-exact": "engineering/vendor-drop/Assembly Notes v2.pdf",
            "suffix-as-version": "engineering/vendor-drop/Assembly Notes",
            "suffix-as-identity": "engineering/vendor-drop/Assembly Notes v2",
            "compare-set": "engineering/vendor-drop/Assembly Notes:set",
        },
        "note": "Space in the name; low-confidence grouping.",
    },
    {
        "path": "engineering/vendor-drop/Assembly Notes v2 FINAL.pdf",
        "content": make_pdf("ASSEMBLY NOTES V2 FINAL"),
        "profiles": {
            "content-path-exact": "engineering/vendor-drop/Assembly Notes v2 FINAL.pdf",
            "suffix-as-version": "engineering/vendor-drop/Assembly Notes",
            "suffix-as-identity": "engineering/vendor-drop/Assembly Notes v2 FINAL",
            "compare-set": "engineering/vendor-drop/Assembly Notes:set",
        },
        "note": "'FINAL' is not a revision scheme. Confidence must land below the "
                "supersedes threshold, so the correct output is compare-set.",
    },

    # --- edge cases that break naive scanners -------------------------------
    {
        "path": "engineering/désign-notes.md",
        "content": "# Désign notes\n\nUnicode path handling.\n".encode("utf-8"),
        "note": "Non-ASCII path. NFC/NFD normalisation differs between macOS and "
                "Windows; the scanner must not treat the two forms as two files.",
    },
    {
        "path": "engineering/widget/.gyst/project.yaml",
        "content": b"name: Widget\nmembers:\n  - engineering/widget/**\n",
        "note": "Membership manifest: precedence 2, above any Gyst suggestion.",
    },
    {
        "path": "engineering/empty.txt",
        "content": b"",
        "note": "Zero bytes. Hashes to the well-known empty sha256; must not be "
                "reported as a duplicate of every other empty file by accident.",
    },
    {
        "path": "engineering/.gystignore",
        "content": b"scratch/\n*.tmp\n",
    },
    {
        "path": "engineering/scratch/tmp-ignore-me.log",
        "content": b"noise\n",
        "ignored": True,
        "note": "Excluded by .gystignore. Must not appear in the inventory at all "
                "-- not even as a path-only entry.",
    },
]

# A real Git repository, so the Git connector has native provenance to read.
GIT_REPO = "firmware"
GIT_COMMITS = [
    {
        "message": "Initial firmware skeleton",
        "files": {
            "README.md": "# Firmware\n",
            "src/main.c": "int main(void) { return 0; }\n",
        },
    },
    {
        "message": "Add watchdog init",
        "files": {
            "src/main.c": "void wdt_init(void);\n\nint main(void) { wdt_init(); return 0; }\n",
        },
    },
]


def sha256(data: bytes) -> str:
    return hashlib.sha256(data).hexdigest()


def build_tree(root: Path) -> dict:
    if root.exists():
        shutil.rmtree(root)
    root.mkdir(parents=True)

    inventory = []
    for spec in FILES:
        p = root / spec["path"]
        p.parent.mkdir(parents=True, exist_ok=True)
        p.write_bytes(spec["content"])
        os.utime(p, (EPOCH, EPOCH))

        entry = {
            "path": spec["path"],
            "size_bytes": len(spec["content"]),
            "sha256": sha256(spec["content"]),
            "mtime_epoch": EPOCH,
            "ignored": spec.get("ignored", False),
        }
        for key in ("generated_from", "duplicate_of", "profiles", "note"):
            if key in spec:
                entry[key] = spec[key]
        inventory.append(entry)

    git = build_git_repo(root / GIT_REPO)

    # The Git repository's working tree holds real files, and a filesystem scan
    # sees them whether or not they were declared above. Leaving them out of the
    # inventory made the expected count disagree with a correct scanner.
    #
    # They are also the first case of one artifact observable through two
    # connectors: local-folder sees bytes at a path, git sees the same bytes at
    # a commit. Both observations are valid and must reconcile rather than
    # compete.
    for rel, text in sorted(git["working_tree"].items()):
        data = text.encode()
        inventory.append({
            "path": f"{GIT_REPO}/{rel}",
            "size_bytes": len(data),
            "sha256": sha256(data),
            "mtime_epoch": None,
            "ignored": False,
            "also_observable_via": "git",
            "note": "Inside a Git working tree. Mtime is not pinned because Git "
                    "writes it; identity must come from content, not mtime.",
        })

    return {
        "$comment": "Generated by testdata/generate.py. Expectations are hand-authored "
                    "in that file; digests and commit ids are computed.",
        "fixture_version": "0.1.0",
        "root": "testdata/tree",
        "counts": {
            "files_total": len(inventory),
            "files_expected_in_inventory": sum(1 for e in inventory if not e["ignored"]),
            "files_expected_ignored": sum(1 for e in inventory if e["ignored"]),
            "duplicate_pairs": sum(1 for e in inventory if "duplicate_of" in e),
            "generated_files": sum(1 for e in inventory if "generated_from" in e),
        },
        "files": sorted(inventory, key=lambda e: e["path"]),
        "git": git,
    }


def build_git_repo(repo: Path) -> dict:
    repo.mkdir(parents=True, exist_ok=True)
    env = dict(os.environ)
    env.update({
        "GIT_AUTHOR_NAME": GIT_AUTHOR[0], "GIT_AUTHOR_EMAIL": GIT_AUTHOR[1],
        "GIT_COMMITTER_NAME": GIT_AUTHOR[0], "GIT_COMMITTER_EMAIL": GIT_AUTHOR[1],
        "GIT_AUTHOR_DATE": GIT_DATE, "GIT_COMMITTER_DATE": GIT_DATE,
        "GIT_CONFIG_GLOBAL": "/dev/null", "GIT_CONFIG_SYSTEM": "/dev/null",
    })

    def git(*args):
        return subprocess.run(
            ["git", "-C", str(repo), *args],
            env=env, check=True, capture_output=True, text=True,
        ).stdout.strip()

    git("init", "-b", "main", "-q")
    commits = []
    for spec in GIT_COMMITS:
        for rel, text in spec["files"].items():
            f = repo / rel
            f.parent.mkdir(parents=True, exist_ok=True)
            f.write_text(text)
        git("add", "-A")
        git("commit", "-q", "-m", spec["message"])
        commits.append({
            "oid": git("rev-parse", "HEAD"),
            "message": spec["message"],
            "changed": sorted(spec["files"]),
        })

    working = {}
    for spec in GIT_COMMITS:
        working.update(spec["files"])

    return {
        "path": GIT_REPO,
        "branch": "main",
        "note": "Commit ids are deterministic: fixed author, committer, and dates. "
                "The Git connector may assert these exactly.",
        "commits": commits,
        "working_tree": working,
    }


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--check", action="store_true",
                    help="rebuild into a temp dir and verify the committed inventory still matches")
    args = ap.parse_args()

    if args.check:
        with tempfile.TemporaryDirectory() as tmp:
            fresh = build_tree(Path(tmp) / "tree")
        fresh["root"] = "testdata/tree"
        if not EXPECTED.exists():
            print("expected-inventory.json missing; run without --check first", file=sys.stderr)
            return 1
        committed = json.loads(EXPECTED.read_text())
        if committed == fresh:
            print("fixture is deterministic and matches expected-inventory.json")
            return 0
        print("MISMATCH: regenerating produced a different inventory", file=sys.stderr)
        return 1

    inventory = build_tree(TREE)
    EXPECTED.write_text(json.dumps(inventory, indent=2, ensure_ascii=False) + "\n")
    c = inventory["counts"]
    print(f"tree:      {TREE}")
    print(f"expected:  {EXPECTED}")
    print(f"files:     {c['files_total']} "
          f"({c['files_expected_in_inventory']} indexed, {c['files_expected_ignored']} ignored)")
    print(f"duplicates: {c['duplicate_pairs']}   generated: {c['generated_files']}")
    print(f"commits:   {len(inventory['git']['commits'])}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
