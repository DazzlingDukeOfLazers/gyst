# Day 7 notes: where a root lives, and finding roots at all

Two things a scan could not say before today: where the folder it was pointed
at physically lives, and which folders are worth pointing it at.

## Location is a policy input

The architecture already gives each kind of place different rules. A network
share gets scheduled, rate-limited passes and may be unavailable. A
cloud-synced folder may hold files whose bytes are not on disk and must never
contain Git metadata. A local folder is fast and complete. None of that can
be applied if the source does not know which it is, so the kind is now
recorded when a source is registered, printed with every scan, and shown as a
column in `gyst status`.

`location.Classify` is a pure function over an `Env` the OS probe fills in:
filesystem type and mount point, the home directory, environment variables
such as `OneDrive`, the roots Dropbox declares in its own `info.json`, and a
way to test for marker files. Every rule is therefore testable on any
platform, and the probes themselves are small: `statfs` on macOS,
`/proc/self/mounts` with a magic-number fallback on Linux, `GetDriveType` and
`GetVolumeInformation` on Windows, plus the UNC prefix.

Precedence is cloud, then network, then local. A synced folder sits on an
ordinary local volume; the sync is what changes the rules, so it has to win
over the filesystem type.

Confidence is honest about the evidence. A root the sync client itself
declares scores 0.95. A well-known folder under home scores 0.85 or 0.9. A
marker file such as `.dropbox` found above the path scores 0.7, because a
copied tree carries markers too. A generic FUSE mount is `unknown` at 0.3
rather than guessed. Nothing known at all is `unknown` at 0. Every outcome
carries an evidence sentence, and a test asserts that none is empty.

Two refusals worth naming. `~/Library/CloudStorage` itself is not a synced
folder; only its per-account children are. And the home directory is never
probed for markers, because `~/.dropbox` is the client's configuration
folder, not a mark on a synced tree. The second one was a failing test before
it was a rule.

## Placeholders are never opened

A file held by a File Provider on macOS carries `SF_DATALESS`; on Windows,
OneDrive Files On-Demand sets `FILE_ATTRIBUTE_RECALL_ON_DATA_ACCESS` and its
relatives. Opening such a file downloads it. A read-only scanner that
triggers downloads has a side effect and an egress, whatever its policy says.

The scanner now checks the stat result before anything else. A placeholder
is observed by metadata only, with the claim payload saying `placeholder:
true`, a warning explaining that the content is not present locally, and no
digest, regardless of the content policy. The logical size is still recorded.
The scan counts them separately.

The test for this hands `observeFile` a fake stat on a path that does not
exist. If the file were opened the call would fail, so a nil error is the
proof that it was not.

## Discovery

`gyst discover` walks the home directory and mounted volumes, or the roots
given, and lists every directory carrying a project marker: `.git`, `go.mod`,
`Cargo.toml`, `package.json`, a KiCad project file, and a dozen others. Each
candidate is classified, and any that is already a registered source is
labelled with its id. It reads directory listings only, never follows a
symlink, prunes dependency caches and build output, and stops at the first
marker unless asked to look inside projects for projects.

On this machine it found 33 candidates in 899 directories in a fifth of a
second, including every repository under `~/personal-git`.

A candidate is not a project. The product's projects are many-to-many with
folders and declared by manifests; a marker walk has no basis for that. What
it produces is a list of places a person might register as sources, and the
JSON output is the "discovery roots" the design's Phase 0 asked for.

## What this does not do

- Nothing has been verified against a live SMB mount, a UNC path, or a real
  OneDrive placeholder. The fstype mapping and the attribute checks are unit
  tested with constructed inputs. The Windows probe compiles and is vetted
  but has not run.
- A source registered before today has `unknown` as its location until it is
  scanned again, which re-registers it.
- Discovery does not persist anything. The report can rerun it.
- No policy acts on the location yet. It is recorded and shown; scheduling,
  rate limits, and the no-Git-in-synced-folders rule are still to come.
