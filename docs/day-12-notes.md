# Day 12 notes: authority

The product's second question is "where is the authority?" The designer's
correction on day 11 drew the line that shaped this: a manifest declares
membership, a profile marks a current version, and neither is authority.
Only a person declares it, or nothing does. So authority arrives in two
parts: a record of what people assert, and a projection that resolves each
file's state from that record first and from weaker evidence after.

## Assertions are durable

`assertions` holds a person's statement about a file: that it is the
authority, or that this copy is not. Each row records who, why, when, and
the observation the person was looking at. Rows are never edited and never
deleted; a trigger enforces both, the way the observation log is enforced.
Changing one's mind is a retraction recorded on the row, with who and why,
so the history of what was asserted stays readable and appears in the
report. Only the `user` actor kind is allowed by a check constraint.

`gyst assert authority <file> --by <name> --reason <text>` records one;
`not-authority` records the other; `retract` withdraws; `list` shows them.

## Four states, in strict precedence

| State | When | Confidence |
|---|---|---|
| declared | Exactly one active authority assertion among a file's peers | 1.0, explicit |
| likely | No assertion; the active profile marks a current member of a multi-member series at 0.8 or above, and nobody has denied it | the profile's |
| multiple | Two conflicting assertions; or a low-confidence series; or byte-identical copies with nothing to choose between them | 0 |
| none | A sole copy nobody declared; or every other copy denied but the survivor never declared | 0 |

Peers are the union of a file's identity group and every present file
with the same digest: two notions of sameness, both consulted. The resolver
is a pure function with a test per rule.

Two refusals worth naming. A `not-authority` on the profile's current member
removes the likely answer and promotes nothing. Denying every copy but one
leaves the survivor at none, with an explanation saying it has not been
declared. Ruling out is not declaring.

## What the fixture showed

Under `suffix-as-identity`, the widget BOM and its copy are multiple
candidates and everything else is none. Under `suffix-as-version`, rev3
becomes the likely authority for the rev2/rev3 series at the profile's 0.88,
and the connector 123/124 trap shows up as multiple candidates, because
that profile groups them at compare-set confidence and a compare-set has
candidates, not a current member. The trap surfacing as ambiguity rather
than as a supersession is the profile and the resolver agreeing with the
day-1 fixture note.

## What this does not do

- Assertions cover authority only. Membership, grouping, and candidate
  resolution assertions are the same shape and the same table; they are
  not wired.
- No authorization. Anyone at the keyboard is `user`. Who may assert is an
  organization policy the design leaves to the deployment.
- Retraction is recorded but not versioned: one retraction per assertion.
- The likely threshold is a constant, 0.8, matching the schema's line
  between an inference and a suggestion.
