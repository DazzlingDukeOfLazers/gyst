#!/usr/bin/env python3
"""Validate the example envelopes against the v0 schemas.

Both directions are checked. Valid examples must pass, and invalid examples must
fail *for the stated reason* -- a case that fails because of an unrelated typo
would otherwise look like a passing test while the constraint it was written to
exercise goes unexercised.

Usage:
    .venv/bin/python schemas/validate.py
"""

import json
import sys
from pathlib import Path

try:
    from jsonschema import Draft202012Validator
    from referencing import Registry, Resource
except ImportError:
    sys.exit("missing deps: python3 -m venv .venv && .venv/bin/pip install jsonschema rfc3339-validator")

HERE = Path(__file__).resolve().parent
SCHEMA_DIR = HERE / "v0"
EXAMPLES = HERE / "examples"

GREEN, RED, DIM, RESET = "\033[32m", "\033[31m", "\033[2m", "\033[0m"


def load_registry():
    """Register every v0 schema by its $id so cross-file $refs resolve offline."""
    resources = []
    for path in sorted(SCHEMA_DIR.glob("*.json")):
        doc = json.loads(path.read_text())
        resources.append((doc["$id"], Resource.from_contents(doc)))
    return Registry().with_resources(resources), {
        path.stem.replace(".schema", ""): json.loads(path.read_text())
        for path in sorted(SCHEMA_DIR.glob("*.json"))
    }


def crosscheck_fixtures(cases) -> list:
    """Every locator an example cites must exist in the fixture tree with the
    digest and size claimed. Without this the examples drift into fiction the
    moment the dataset changes, and they stop being evidence of anything.

    A case may exempt specific locators via allow_absent_locators. The rename
    example needs one: a move's destination is a path that exists only after
    the move, so a static tree cannot contain it. Exemptions are per-locator
    and must be declared, so the guard stays strict everywhere it can be."""
    inventory_path = HERE.parent / "testdata" / "expected-inventory.json"
    if not inventory_path.exists():
        return [("testdata", "expected-inventory.json missing; run testdata/generate.py")]

    inv = json.loads(inventory_path.read_text())
    by_path = {e["path"]: e for e in inv["files"]}
    problems = []

    def walk(node, path_hint):
        """Find every ArtifactRef-shaped object anywhere in an instance."""
        if isinstance(node, dict):
            loc = node.get("location")
            if isinstance(loc, dict) and "locator" in loc and node.get("kind") == "file":
                locator = loc["locator"]
                entry = by_path.get(locator)
                if entry is None:
                    problems.append((path_hint, f"cites {locator!r}, absent from the fixture tree"))
                else:
                    ver = node.get("version") or {}
                    size = ver.get("size_bytes")
                    if size is not None and size != entry["size_bytes"]:
                        problems.append((path_hint, f"{locator}: size {size} != fixture {entry['size_bytes']}"))
                    digest = (ver.get("content_digest") or {}).get("hex")
                    if digest is not None and digest != entry["sha256"]:
                        problems.append((path_hint, f"{locator}: digest does not match the fixture"))
            for value in node.values():
                walk(value, path_hint)
        elif isinstance(node, list):
            for value in node:
                walk(value, path_hint)

    exempt = {
        c["file"]: set(c.get("allow_absent_locators", []))
        for c in cases
    }
    for case_file in sorted(EXAMPLES.rglob("*.json")):
        if case_file.name == "cases.json":
            continue
        rel = case_file.relative_to(EXAMPLES).as_posix()
        allowed = exempt.get(rel, set())
        before = len(problems)
        walk(json.loads(case_file.read_text()), rel)
        # Drop problems that name an explicitly exempted locator.
        problems[before:] = [
            (where, msg) for where, msg in problems[before:]
            if not any(loc in msg for loc in allowed)
        ]
    return problems


def main() -> int:
    registry, schemas = load_registry()
    cases = json.loads((EXAMPLES / "cases.json").read_text())["cases"]

    failures = []
    for case in cases:
        name = case["file"]
        schema = schemas[case["schema"]]
        instance = json.loads((EXAMPLES / name).read_text())
        validator = Draft202012Validator(
            schema, registry=registry,
            format_checker=Draft202012Validator.FORMAT_CHECKER,
        )
        errors = sorted(validator.iter_errors(instance), key=lambda e: str(e.json_path))

        if case["expect"] == "valid":
            if errors:
                failures.append((name, "expected valid, got: " + errors[0].message))
                print(f"{RED}FAIL{RESET} {name}")
                for e in errors:
                    print(f"       {e.json_path}: {e.message}")
            else:
                print(f"{GREEN}ok  {RESET} {name}")
        else:
            if not errors:
                failures.append((name, "expected invalid, but it validated"))
                print(f"{RED}FAIL{RESET} {name}  {DIM}(accepted a record that violates: "
                      f"{case['violates']}){RESET}")
                continue
            blob = " ".join(e.message for e in errors)
            needle = case.get("expect_error_contains")
            if needle and needle not in blob:
                failures.append((name, f"rejected, but not for the stated reason "
                                       f"(wanted {needle!r}, got: {blob[:120]})"))
                print(f"{RED}FAIL{RESET} {name}  {DIM}rejected for the wrong reason{RESET}")
                for e in errors:
                    print(f"       {e.json_path}: {e.message}")
            else:
                print(f"{GREEN}ok  {RESET} {name}  {DIM}rejected: {case['violates']}{RESET}")

    print()
    drift = crosscheck_fixtures(cases)
    for where, problem in drift:
        print(f"{RED}DRIFT{RESET} {where}: {problem}")
    if drift:
        print()

    if failures or drift:
        print(f"{RED}{len(failures)} of {len(cases)} cases failed, "
              f"{len(drift)} fixture mismatches{RESET}")
        return 1
    valid_n = sum(1 for c in cases if c["expect"] == "valid")
    print(f"{GREEN}all {len(cases)} cases pass{RESET} "
          f"({valid_n} accepted, {len(cases) - valid_n} correctly rejected); "
          f"every cited locator matches the fixture tree")
    return 0


if __name__ == "__main__":
    sys.exit(main())
