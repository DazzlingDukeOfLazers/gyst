# ADR 006: Signed transfer bundles

Status: proposed, 2026-09-20, with a first implementation behind it.
Decision requested from Daniel; the format is versioned `gyst.bundle/0.1.0`
and can change before anyone depends on it.

## Context

The third goal of this round was that a diaspora of Gyst users can
contribute, report, and exchange evidence without a shared server, with
files as messages. The architecture already owed "signed, inspectable
transfer bundles" for the air-gapped profile, ADR 012 on the list, and
ADR 005 needs the same writer for archiving compacted history. One format
serves all three.

Two constitutional constraints bear on it. Customer data does not leave
its authorized boundary: every observation carries an egress level, and a
bundle is precisely the act of data leaving. And every derived claim keeps
its evidence: a bundle must carry observations in the schema they were
recorded in, ids intact, so that anything citing them still resolves.

## Decision (proposed)

**Format.** A JSON Lines file. Line one is a header: schema id, bundle id,
creation time, the sender's id and Ed25519 public key, the declared egress
level, the sources the observations belong to, counts, the SHA-256 of
everything after the first newline, and a signature. Every following line
is one observation envelope in the v0 schema, exactly as the sender's log
held it. A text editor can read it and the public key alone can verify it.

**Signature.** Ed25519 from Go's standard library, over the header with
its signature field empty, canonically marshalled, which includes the body
digest. Verification recomputes the digest and checks the signature before
anything else is looked at. A bundle that fails either is refused whole.

**Egress.** An observation records the level at which it may travel:
`device`, `facility`, or `connected-server`, set at scan time and
defaulting to `device`. An export declares its destination level and
includes only observations whose recorded level permits it; the rest are
counted as withheld and the count is shown. Nothing recorded as `device`
ever leaves through a bundle. This is the boundary constraint made
mechanical, and it is why the default stays `device`.

**Trust.** The receiver keeps a table of trusted senders and their keys.
An unknown sender is refused unless the person importing says
`--trust-on-first-use` and gives their name, which records the key with
that provenance. A known sender whose key differs is refused outright;
replacing a key is a deliberate act done by hand. Trust is per receiver
and never transitive.

**Namespace and relabel.** Imported sources are registered as
`<sender>/<source>` so two Gysts that both call a source "engineering"
do not collide, and are marked with the sender. Visibility labels on
imported observations are replaced by one naming the sender, because the
sender's labels mean nothing to the receiver's authorization. Observation
ids are kept, so the sender's relations, findings, and assertions cite
evidence that resolves.

**No forwarding.** A bundle carries what its sender observed. Sources that
were themselves imported are never exported, and naming one is an error.
Forwarding another sender's evidence under one's own signature would
misattribute it.

**Only observations, for now.** Findings, relations, and authority are
recomputed by the receiver from the imported evidence, alongside its own.
That is correct and it is also the interesting result: a file the sender
has and the receiver also has shows up as a duplicate across two Gysts.
Assertions are a person's statements and belong in a bundle eventually,
attributed to sender and actor; they wait for a reason.

## Acceptance

- A bundle written by one store and read by another yields the same
  observations, ids intact, under the namespaced source.
- A changed byte in the body fails the digest; a changed header fails the
  signature; a different key claiming the sender's id fails the signature
  and, if the id is known, the trust check.
- An observation recorded at `device` never appears in a bundle whose
  declared egress is `facility` or beyond.
- Importing the same bundle twice appends nothing the second time.
- An imported source cannot be exported.

All five hold in the store suite and were exercised through the command
line between two SQLite stores.

## Consequences

- Two tables, `trusted_keys` and `bundles`, and one column,
  `sources.imported_from`, on both engines.
- Four commands: `gyst key`, `gyst trust`, `gyst export`, `gyst import`,
  and an `--egress` flag on `scan` and `git`.
- A private key is one file in the data directory, owner-readable only.
  Losing it means a new identity; there is no recovery and no escrow.
- The archive writer ADR 005 needs is this writer with a different
  selection of observations.

## Not decided here

- Key rotation and revocation. A sender with a new key is a new sender to
  every receiver until each one decides otherwise.
- Bundles of assertions, findings, or a release manifest.
- Transport. A bundle is a file; how it travels is not Gyst's concern.
