# Sample reports

`fixture-report.json` is the output of `gyst report` against a clean
database holding only the synthetic fixture in `testdata/`: one local-folder
source scanned with a 7-day cadence, one Git source, the `suffix-as-identity`
profile applied, and one authority assertion so the declared state appears. It is the data contract for the static report described in
[`design-system.md`](../design-system.md) and the "representative generated
JSON" its Phase 0 asks for.

Regenerate it with:

```sh
createdb gyst_fixture && for f in migrations/*.sql; do psql -q -d gyst_fixture -f "$f"; done
export GYST_DATABASE_URL=postgres:///gyst_fixture
python3 testdata/generate.py
gyst scan --root testdata/tree --source src_local_eng_share --cadence 7d
gyst git  --repo testdata/tree/firmware --source src_git_firmware
gyst identity apply --profile suffix-as-identity
gyst assert authority widget_bom.xlsx --by example --reason "the workbook the release is built from"
gyst report --out docs/samples/fixture-report.json
```

The home directory in `root` paths has been replaced with `/home/example`.
Timestamps are whatever the clock said; nothing else is edited.
