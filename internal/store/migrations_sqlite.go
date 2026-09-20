package store

import _ "embed"

//go:embed schema_sqlite.sql
var sqliteSchema string

//go:embed schema_sqlite_0002.sql
var sqliteBundles string

// sqliteMigrations are applied in order on open, once each. Later entries
// are deltas, like the PostgreSQL migrations; the first is the schema as
// it stood when SQLite arrived.
var sqliteMigrations = []struct{ name, body string }{
	{"0001_schema", sqliteSchema},
	{"0002_bundles_and_trust", sqliteBundles},
}
