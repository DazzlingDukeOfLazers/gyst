# Envelope schemas (v0)

These are the four contracts named in the roadmap's first backlog. They exist to
turn invariants that `architecture.md` and `design-round-2.md` state in prose
into constraints a machine rejects.

Status is pre-alpha. `v0` is unstable by definition; the version exists so that
the first breaking change is a visible event rather than a silent reinterpretation.

| Schema | Role |
|---|---|
| `common.schema.json` | Shared primitives. Not a standalone document. |
| `artifact-ref.schema.json` | Reference to something Gyst has seen. |
| `observation.schema.json` | Immutable statement about source state at a point in time. |
| `relation.schema.json` | Typed link between two artifacts, derived from evidence. |
| `finding.schema.json` | A policy rule and the evidence it matched. |

## The load-bearing distinction

`ArtifactRef` separates three things the prose treats as one word:

- **Location** — where a version was physically seen. A raw fact.
- **version** — the observed state of the content itself, identified by digest.
- **artifact** — the logical grouping an identity profile assigned.

Only the first two may appear on an `Observation`. The `artifact` block is a
projection, and the observation schema actively rejects its presence on an
evidence record. This is what makes the reversibility claim in `design-round-2.md`
mechanically true: switching identity profiles rebuilds groupings without
touching a single stored observation, because no observation ever contained one.

## Constraints that bite

Four rules from the design documents are enforced rather than described:

1. **Grouping is never evidence.** An observation carrying `subject.artifact` is
   rejected.
2. **Policy governs content.** A `content_digest` under `exclude` or `metadata`
   policy is rejected: Gyst may not record a digest it was not permitted to compute.
3. **No supersession from a suffix.** A machine-suggested `supersedes` requires an
   explanation and confidence of at least 0.8. Below that the correct output is
   `compare-set-with`.
4. **Derived records cite evidence.** `evidence` is required and non-empty on
   every `Relation` and `Finding`, and only a human may waive a finding.

## Running the validator

```bash
python3 -m venv .venv && .venv/bin/pip install jsonschema rfc3339-validator
python3 testdata/generate.py
.venv/bin/python schemas/validate.py
```

The validator checks both directions. Examples in `examples/valid/` must pass;
examples in `examples/invalid/` must fail *for the reason declared* in
`examples/cases.json`. An invalid example that fails for an unrelated reason is
reported as a failure, because it would otherwise leave its constraint untested.

It also cross-checks every file locator cited by an example against
`testdata/expected-inventory.json`, so the examples cannot drift into describing
files that no longer exist or digests that no longer match.

## Known gaps

- `claim.payload` is unconstrained. Payload shape belongs to the extractor's
  declared `output_schema`, which does not exist yet.
- Visibility labels are opaque strings. The mapping from source principals to
  labels is ADR-007 and is not designed.
- No schema covers `GenerationRun` or `Release`. Both are Phase 2.
