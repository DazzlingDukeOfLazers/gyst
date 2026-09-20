# Day 16 notes: no server required

Stage four of ADR 004, and the smallest of the four: with
`GYST_DATABASE_URL` unset, Gyst opens a SQLite file in the user's data
directory. `~/Library/Application Support/gyst/gyst.db` on macOS,
`~/.local/share/gyst/gyst.db` on Linux, `%AppData%\gyst\gyst.db` on
Windows. `GYST_DATA_DIR` moves it. Nothing else changed.

What changed is what the README can now say. Its first section is a
getting-started block that is true: clone, build, discover, scan, explain,
report. No database server, no service, no client tools, no account. That
was the third goal of this round, that a diaspora can pull the repository
and run their own Gyst, and it was the second of the constitutional
constraints, that core operation requires no server. Both were false in
the code until today.

## The acceptance test, run

The ADR's acceptance test is a clean machine with Go and no services:
clone, build, scan, report. Simulated here with an empty home directory,
no environment, and the binary built from this branch: the scan ran, the
status command printed the file it had created under that home, the
report came out, and replay verified. The PostgreSQL databases on this
machine were not touched.

## What this does not do

- The Git connector still shells out to `git`, so a machine with no Git
  can scan folders but not repositories. That is a separate decision.
- There is no migration between engines. A team that starts on a file and
  moves to a server re-scans; the log is the sources, and the sources are
  still there.
- The data directory is per user, which is right for the solo profile and
  wrong for a shared machine. Sharing is what the server profile is for.
