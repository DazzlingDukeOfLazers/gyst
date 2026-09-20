# Day 21 notes: files as messages

The round's third goal, stated at the start as "a way for a diaspora to
contribute, report, and pull/build their own local gyst. Perhaps even
deliver files as messages. Something something security." Building
without a server landed on day 16. This is the messages, and the
something.

`gyst key init alice` makes a signing identity. `gyst export --key alice
--egress facility --out eng.jsonl` writes a bundle of this Gyst's
observations. `gyst import eng.jsonl --trust-on-first-use --by bob`
verifies it, records alice's key, registers her sources as `alice/…`,
and appends her observations to bob's log. Bob's next report shows her
files beside his, and where the two Gysts hold the same bytes, a
duplicate finding across two sources. ADR 006 records the format and the
reasoning.

## The security, specifically

Three things had to be true and each is enforced by code, not by
documentation.

Nothing leaves that was not allowed to. Every observation carries an
egress level, `device` by default. Export declares its destination and
includes only what permits it; the withheld count is printed so the
person sees what stayed. The fixture scanned at `device` exports nothing
to a `facility` bundle, and the test proves it. So does the command line:
scanning one source at facility and another at device, the export
carries the first and withholds the second.

Nothing is believed that was not signed by someone chosen. The body is
digested, the header signed, and a bundle that fails either check is
refused before a line of it is read as data. A sender unknown to the
receiver is refused unless a named person trusts them on first use. A
known sender with a different key is refused with no override flag,
because that is the case the flag would be used wrongly on.

Nothing is misattributed. Imported sources are namespaced by sender and
marked as imported. Visibility labels are replaced by one naming the
sender. Imported sources cannot be re-exported; a bundle carries what its
sender saw and nothing else.

## What surprised

The store's export method rebuilds an observation envelope from its
columns, and that round trip is exactly what the day-2 walking skeleton
never had to prove: that a stored observation can become the JSON it
came from. It can, and the schema examples' validator would accept it.

Namespacing changes the source id inside an observation but keeps its id,
which was derived from the original source id. Nothing re-derives ids for
imported sources, since nobody scans them locally, so the seam is safe,
but it is a seam and the ADR names it.

## What this does not do

- No key rotation or revocation.
- Only observations travel. Findings and authority are recomputed by the
  receiver; assertions do not travel yet.
- A bundle is loaded into memory whole on import. A hundred-thousand-file
  bundle is around a hundred megabytes; fine on a laptop, worth streaming
  later.
- The private key has no passphrase. The file is owner-readable only and
  that is the extent of it.
